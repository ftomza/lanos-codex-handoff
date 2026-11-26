# M5.2 Android Vgate check

M5.1 runs the simulator on the Teclast P85. M5.2 adds a foreground adapter
check on the same device. A link on the Android dashboard opens a native
screen. A separate button explicitly starts the check. It shows each stage,
an adapter identity or an actionable error, and allows cancellation.

Use Android BluetoothGatt behind a Kotlin state machine. The project brief
already selects native GATT for a standalone application. A Mac relay would
require another host, and a browser bridge would give the WebView unnecessary
Bluetooth access. The simulator and its WebMessage feed remain independent.

Scan for the exact advertised name IOS-Vlink. Stop scanning before connecting.
Discover service 18f0, check Notify on 2af0 and Write or Write Without Response
on 2af1, and wait for a successful CCCD write. Use Write Without Response when
advertised, consistent with the Go characteristic API, or acknowledged Write
when that is the only advertised mode. Never switch modes after a failed write.

The only transmitted payload is ASCII ATI followed by CR. Serialize channel
setup and the command, wait for both the write callback and an ELM prompt,
and cap the response at 4096 bytes. Ignore echo and blank lines, retain the
identity, reject empty/error replies. Close the link on success, failure,
cancel, or activity stop. Allow at most one retry after a transport failure,
always after the old scan or GATT connection is closed. Time out each stage.

Request scan and connect permissions on Android 12 and later, or fine location
for legacy Android scanning. Hardware acceptance requires owner approval and
an actual IOS-Vlink ATI result. No ECU polling or live dashboard in M5.2.
