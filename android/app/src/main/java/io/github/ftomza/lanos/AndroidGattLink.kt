package io.github.ftomza.lanos

import android.annotation.SuppressLint
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothProfile
import android.bluetooth.BluetoothStatusCodes
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.SystemClock
import java.time.Instant
import java.util.UUID

// ProbeActivity checks runtime permissions before opening the link. Revocation
// during an operation is reported as a terminal error.
@SuppressLint("MissingPermission")
internal class AndroidGattLink(
    private val context: Context,
    private val adapter: BluetoothAdapter,
    private val main: Handler,
    private val listener: (GattEvent) -> Unit,
) : GattLink {
    private var active = false
    private var scanner: BluetoothLeScanner? = null
    private var gatt: BluetoothGatt? = null
    private var notify: BluetoothGattCharacteristic? = null
    private var tx: BluetoothGattCharacteristic? = null
    private var cccd: BluetoothGattDescriptor? = null
    private var writeMode = BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT

    private val scanCallback = object : ScanCallback() {
        override fun onScanResult(callbackType: Int, result: ScanResult) {
            if (result.scanRecord?.deviceName != DEVICE_NAME) return
            main.post {
                if (active && scanner != null) connect(result.device)
            }
        }

        override fun onScanFailed(errorCode: Int) {
            main.post {
                if (active && scanner != null) {
                    val retryable = errorCode in retryableScanErrors
                    failure("Ошибка поиска Bluetooth ($errorCode).", retryable)
                }
            }
        }
    }

    @Suppress("DEPRECATION")
    private val gattCallback = object : BluetoothGattCallback() {
        override fun onConnectionStateChange(link: BluetoothGatt, status: Int, newState: Int) = dispatch(link) {
            when {
                status != BluetoothGatt.GATT_SUCCESS ->
                    failure("Соединение BLE прервано (GATT $status).", retryableGattError(status))
                newState == BluetoothProfile.STATE_DISCONNECTED ->
                    failure("IOS-Vlink отключился.", retryable = true)
                newState == BluetoothProfile.STATE_CONNECTED -> {
                    listener(GattEvent.Connected)
                    if (!link.discoverServices()) failure("Не удалось начать поиск сервиса BLE.", retryable = true)
                }
            }
        }

        override fun onServicesDiscovered(link: BluetoothGatt, status: Int) = dispatch(link) {
            if (status == BluetoothGatt.GATT_SUCCESS) subscribe(link)
            else failure("Поиск сервиса завершился ошибкой (GATT $status).", retryableGattError(status))
        }

        override fun onDescriptorWrite(link: BluetoothGatt, descriptor: BluetoothGattDescriptor, status: Int) =
            dispatch(link) {
                if (descriptor !== cccd) return@dispatch
                if (status == BluetoothGatt.GATT_SUCCESS) listener(GattEvent.Subscribed)
                else failure("Адаптер отклонил подписку BLE (GATT $status).", retryableGattError(status))
            }

        override fun onCharacteristicWrite(link: BluetoothGatt, characteristic: BluetoothGattCharacteristic, status: Int) =
            dispatch(link) {
                if (characteristic !== tx) return@dispatch
                if (status == BluetoothGatt.GATT_SUCCESS) listener(GattEvent.Written)
                else failure("Не удалось отправить команду ELM (GATT $status).", retryableGattError(status))
            }

        override fun onCharacteristicChanged(
            link: BluetoothGatt,
            characteristic: BluetoothGattCharacteristic,
            value: ByteArray,
        ) {
            val bytes = value.copyOf()
            val receipt = Receipt(SystemClock.elapsedRealtime(), Instant.now())
            dispatch(link) {
                if (characteristic === notify) listener(GattEvent.Bytes(bytes, receipt))
            }
        }

        @Deprecated("Callback for Android 12 and older")
        override fun onCharacteristicChanged(link: BluetoothGatt, characteristic: BluetoothGattCharacteristic) {
            if (Build.VERSION.SDK_INT < 33) {
                characteristic.value?.let { onCharacteristicChanged(link, characteristic, it) }
            }
        }
    }

    override fun start() {
        active = true
        withPermission {
            if (!adapter.isEnabled) {
                failure("Bluetooth выключен.", retryable = true)
                return@withPermission
            }
            val available = adapter.bluetoothLeScanner
            if (available == null) {
                failure("Сканер Bluetooth LE недоступен.", retryable = true)
                return@withPermission
            }
            scanner = available
            val target = ScanFilter.Builder().setDeviceName(DEVICE_NAME).build()
            val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
            available.startScan(listOf(target), settings, scanCallback)
        }
    }

    private fun connect(device: BluetoothDevice) = withPermission {
        stopScan()
        listener(GattEvent.Found)
        gatt = device.connectGatt(context, false, gattCallback, BluetoothDevice.TRANSPORT_LE)
        if (gatt == null) failure("Не удалось открыть соединение с IOS-Vlink.", retryable = true)
    }

    private fun subscribe(link: BluetoothGatt) {
        val service = link.getService(SERVICE)
        val rx = service?.getCharacteristic(NOTIFY)
        val writer = service?.getCharacteristic(TX)
        if (rx == null || writer == null) {
            failure("У IOS-Vlink отсутствует сервис 18F0 или его каналы 2AF0/2AF1.")
            return
        }
        val descriptor = rx.getDescriptor(CCCD)
        val mode = when {
            writer.properties and BluetoothGattCharacteristic.PROPERTY_WRITE_NO_RESPONSE != 0 ->
                BluetoothGattCharacteristic.WRITE_TYPE_NO_RESPONSE
            writer.properties and BluetoothGattCharacteristic.PROPERTY_WRITE != 0 ->
                BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT
            else -> null
        }
        if (rx.properties and BluetoothGattCharacteristic.PROPERTY_NOTIFY == 0 || descriptor == null || mode == null) {
            failure("Сервис IOS-Vlink не поддерживает ожидаемые Notify, CCCD и Write.")
            return
        }
        notify = rx
        tx = writer
        cccd = descriptor
        writeMode = mode
        listener(GattEvent.Discovered)
        if (!link.setCharacteristicNotification(rx, true) || !writeSubscription(link, descriptor)) {
            failure("Не удалось включить уведомления IOS-Vlink.")
        }
    }

    @Suppress("DEPRECATION")
    private fun writeSubscription(link: BluetoothGatt, descriptor: BluetoothGattDescriptor): Boolean {
        val bytes = BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE
        if (Build.VERSION.SDK_INT >= 33) {
            return link.writeDescriptor(descriptor, bytes) == BluetoothStatusCodes.SUCCESS
        }
        return descriptor.setValue(bytes) && link.writeDescriptor(descriptor)
    }

    @Suppress("DEPRECATION")
    override fun write(command: ElmCommand) = withPermission {
        val link = gatt
        val writer = tx
        if (link == null || writer == null) {
            listener(GattEvent.WriteRejected("Канал отправки команд ELM недоступен."))
            return@withPermission
        }
        val bytes = command.bytes()
        if (Build.VERSION.SDK_INT >= 33) {
            val status = link.writeCharacteristic(writer, bytes, writeMode)
            if (status != BluetoothStatusCodes.SUCCESS) {
                listener(GattEvent.WriteRejected("Android отклонил отправку ${command.wire} ($status).", retryableGattWrite(status)))
            }
            return@withPermission
        }
        writer.writeType = writeMode
        if (!writer.setValue(bytes) || !link.writeCharacteristic(writer)) {
            listener(GattEvent.WriteRejected("Android отклонил отправку ${command.wire}."))
        }
    }

    override fun close() {
        active = false
        runCatching { stopScan() }
        val old = gatt
        gatt = null
        if (old != null) {
            runCatching { old.disconnect() }
            runCatching { old.close() }
        }
    }

    private fun stopScan() {
        val old = scanner
        scanner = null
        old?.stopScan(scanCallback)
    }

    private fun dispatch(link: BluetoothGatt, action: () -> Unit) {
        main.post {
            if (active && gatt === link) withPermission(action)
        }
    }

    private fun withPermission(action: () -> Unit) {
        try {
            action()
        } catch (_: SecurityException) {
            failure("Android отозвал доступ к Bluetooth. Проверьте разрешения приложения.")
        } catch (_: IllegalStateException) {
            failure("Bluetooth временно недоступен.", retryable = true)
        }
    }

    private fun failure(detail: String, retryable: Boolean = false) {
        listener(GattEvent.Failed(detail, retryable))
    }

    private companion object {
        const val DEVICE_NAME = "IOS-Vlink"
        val retryableScanErrors = setOf(
            ScanCallback.SCAN_FAILED_APPLICATION_REGISTRATION_FAILED, ScanCallback.SCAN_FAILED_INTERNAL_ERROR,
            ScanCallback.SCAN_FAILED_OUT_OF_HARDWARE_RESOURCES, ScanCallback.SCAN_FAILED_SCANNING_TOO_FREQUENTLY,
        )
        fun uuid(id: String): UUID = UUID.fromString("0000$id-0000-1000-8000-00805f9b34fb")
        val SERVICE = uuid("18f0")
        val NOTIFY = uuid("2af0")
        val TX = uuid("2af1")
        val CCCD = uuid("2902")
    }
}

// HCI link timeout, peer or host termination, establishment failure,
// Android's generic GATT errors and connection congestion/timeout.
internal fun retryableGattError(status: Int): Boolean = when (status) {
    8, 19, 22, 62, 133, BluetoothGatt.GATT_CONNECTION_CONGESTED, BluetoothGatt.GATT_CONNECTION_TIMEOUT, BluetoothGatt.GATT_FAILURE -> true
    else -> false
}

// BluetoothGatt documents ERROR_DEVICE_NOT_CONNECTED for immediate writes.
// BluetoothStatusCodes uses 4 but does not expose its symbol in the public SDK.
private const val WRITE_DEVICE_NOT_CONNECTED = 4

@android.annotation.TargetApi(Build.VERSION_CODES.TIRAMISU)
internal fun retryableGattWrite(status: Int): Boolean =
    status == WRITE_DEVICE_NOT_CONNECTED || status == BluetoothStatusCodes.ERROR_BLUETOOTH_NOT_ENABLED ||
        status == BluetoothStatusCodes.ERROR_GATT_WRITE_REQUEST_BUSY
