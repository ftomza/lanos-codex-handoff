# Lanos Dashboard for Android

M5.1 packages the existing Lanos instruments in a fullscreen landscape WebView.
The Kotlin simulator runs on the device. The app shows **SIMULATOR**, speed, RPM,
coolant temperature, adapter voltage, timestamps and connection status.
It runs offline. M5.2 adds a separate Vgate adapter check that requests
Bluetooth access when started. M5.3–M5.5 add live instruments, ignition
recovery and a local fuel computer.

## Build and test

Use JDK 17 or newer with the Gradle wrapper. The verified macOS setup uses the
JBR 25 bundled with Android Studio, Android SDK Platform 37 and Build Tools
36.0.0. Gradle 9.6.0 is pinned with its distribution checksum, AGP 9.5.1 includes
Kotlin 2.2.10, and the app compiles to JVM 17 bytecode.

The SDK floor is API 26 for `java.time` and platform WebView messaging. The app
also needs a current Android System WebView for the shared JavaScript modules.
It targets API 35 for the Android 15 tablet and compiles against the installed
API 37. Android can override an app's landscape request on large displays. In
that case rotate the device or set system rotation to landscape.

On macOS, point the shell at the JDK and SDK installed with Android Studio.

```sh
export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
export ANDROID_HOME="$HOME/Library/Android/sdk"
```

Run these commands from the repository root.

```sh
./android/gradlew -p android :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
node --test android/tests/*.test.mjs
```

The signed debug APK is at
[`app/build/outputs/apk/debug/app-debug.apk`](app/build/outputs/apk/debug/app-debug.apk).
The first build downloads Gradle and build dependencies. Runtime assets come from
`web/` at build time. The Android package adds its own stylesheet and feed.

## Run on an emulator

Start an AVD in Android Studio Device Manager or with the SDK emulator. An
Android 15 or newer system image with an up-to-date WebView is suitable.

```sh
"$ANDROID_HOME/emulator/emulator" -list-avds
"$ANDROID_HOME/emulator/emulator" -avd Medium_Phone_API_37.0
```

Use your AVD name if it differs. In a second terminal, select the emulator
with `adb -e` to install and start the app.

```sh
adb -e install -r android/app/build/outputs/apk/debug/app-debug.apk
adb -e shell am start -n io.github.ftomza.lanos.debug/io.github.ftomza.lanos.MainActivity
```

For a 1280×800 landscape test using a portrait phone AVD, set the following on
that emulator before launching the app.

```sh
adb -e shell wm size 800x1280
adb -e shell wm density 160
adb -e shell wm user-rotation lock 1
```

Reset with `wm size reset`, `wm density reset` and `wm user-rotation free` on
the same emulator.

Check that the label reads **SIMULATOR** and the instruments change over the
60-second cycle. Voltage is unavailable at seconds 24–28. Instruments show
dashes during stale data at seconds 42–48 and connection loss at seconds 48–54.
They recover automatically. Put the app in the background and reopen it to
check that readings resume with fresh timestamps. The exact emulator evidence
and remaining tablet checks are in [STATUS.md](../docs/STATUS.md).

## Check Vgate on Teclast P85

Install the debug APK from the repository root with the tablet connected
and USB debugging authorized.

```sh
adb -d install -r android/app/build/outputs/apk/debug/app-debug.apk
adb -d shell am start -n io.github.ftomza.lanos.debug/io.github.ftomza.lanos.MainActivity
```

The debug build is named **Lanos LIVE**. It upgrades **Lanos BLE Check**
and can be installed next to the earlier **Lanos Simulator**. Keep Android's
default debug keystore for future updates to the debug app.

For the agreed physical check, park the car, power the Vgate iCar Pro and
close any other OBD clients. The engine can stay off. Turn Bluetooth on in
Android, then open **Проверить Vgate** from the dashboard and tap
**Проверить адаптер**. On Android 12 and later, allow access to Nearby devices.
On Android 8–11, enable location services and allow location access for the
scan. The adapter need not be paired manually.

Success is **Ответ ATI от IOS-Vlink** followed by the version string expected
from this adapter, such as `ELM327 v2.3`. The app closes the link after the
check. The cancellation button and leaving the screen also stop it. If the
check fails, keep the exact message to identify whether the failure occurred
during scanning, GATT setup or ATI. The check reads the adapter identity.

## First live dashboard check

With the car parked, close other OBD clients, power Vgate and switch on the
ignition. In **Lanos LIVE**, tap **Подключить авто** and grant Bluetooth access
if prompted. Look for **LIVE**, a zero RPM reading with the engine off,
coolant temperature in °C, and adapter supply voltage in V. When safe to
start the engine while parked, verify that RPM updates. Speed is displayed
when advertised by the ECU. The adapter voltage is from `ATRV` and reflects
supply at the OBD adapter.

Dashes indicate missing or expired measurements. In **Ожидаем ЭБУ** the app
can still show a fresh adapter voltage. It checks for an ECU reply about every
10 seconds after the previous check completes. When the adapter is missing,
the app shows **Ожидаем адаптер** and retries on its own while LIVE is open.
Leaving the app or turning off the screen closes the connection. When the
dashboard comes back to the foreground in LIVE, it starts a new connection
automatically. Tap **Симулятор** to switch back to demonstration data and
stop reconnecting.
The dashboard keeps the screen on while it is visible. When you leave it,
Android uses the normal screen timeout. The tablet's power button can still
turn off the display.

After an interrupted command, a new BLE subscription listens for the ELM
prompt for eight seconds. A late prompt resumes polling automatically.
If the app knows the interrupted command was different from `ATI`, it
then tries one adapter version request. A confirmed write and a complete
version reply resume initialization. The journal records the attempt and
the result as `vgate_ati_recovery`. After a cold restart with an unknown
previous command, the app waits for the prompt.

If Vgate still requires manual recovery, unplug it while parked until its
power light goes out, plug it back in, and use **Питание Vgate отключал**
in the status panel. That confirmation starts the new connection.

## Check ignition and BLE recovery

1. Park, close any other OBD clients, turn ignition on and open **Подключить авто**.
   Wait for RPM, coolant and adapter voltage. A real zero RPM with the engine off is valid.
2. Keep the app open and switch ignition off. If the adapter stays powered and
   the ECU stops answering, look for **Ожидаем ЭБУ**, dashes for ECU readings
   and adapter voltage if fresh. If the adapter goes offline, look for
   **Ожидаем адаптер** and a retry message.
3. Turn ignition back on and watch for fresh readings. The app may take
   several seconds for a new check or connection. If it reports a missing ELM
   prompt, follow the app's power-cycle recovery dialog.
4. With LIVE open, turn Android Bluetooth off and on. Check that readings
   return automatically. Next, turn ignition off and put the tablet to sleep
   while Vgate stays plugged in. Wake it and start the engine. Check whether
   readings return without using the recovery button. Export both logs so
   that the prompt and ATI paths can be distinguished.
5. After the check, switch to **Симулятор** or leave the app to stop polling.

Supported MAP, IAT and bank 1 fuel trims have their own timestamps. The main
screen shows speed, RPM, coolant and voltage alongside the fuel panel.

## Проверить топливный компьютер

- Открой **Симулятор**. На месте мгновенный расход показан в л/ч, с 5 км/ч
  переключается на л/100 км. Средний появляется после 0,1 км с совместным
  покрытием топлива и скорости. Во время имитации обрыва мгновенный расход
  исчезает, итоги сохраняются.
- Для проверки на припаркованной машине открой **Подключить авто** и дождись
  показаний. При работающем двигателе LIVE-расход сначала отключён. В блоке
  **Топливный компьютер** открой **Настройки → Оценка LIVE по IAT → Включить**.
  При свежих RPM, MAP и IAT появится оценка с отметкой о непроверенном IAT.
  Свежие нулевые обороты дают нулевой расход. Стояночная проверка подтверждает
  наличие расчёта, а точность остаётся открытой.
- Для нового отсчёта после появления свежих показаний выбери **Настройки →
  Сброс → Обнулить поездку**. Поправка сохраняется, если её явно не вернуть к ×1.
  У SIM, обычного LIVE и экспериментального LIVE отдельные локальные итоги.
  Перезапуск и потеря данных отмечают покрытие как частичное.
- Для калибровки введи фактически залитые литры и подтверди совпадение границ
  от полного до полного с текущим отсчётом. Нужны полное покрытие и минимум
  10 км. Проверь рассчитанную поправку перед применением. В том же диалоге
  доступны отмена последней поправки и возврат к ×1.

В Android расчёт использует допущения Go о 1,5 л, VE 0,75, AFR 14,7 и плотности
745 г/л. Общие [контрольные примеры](../internal/fuel/testdata/android-golden.json)
проверяют расход в л/ч, сырые литры, километры и время в Go и Kotlin с
абсолютным допуском `1e-9` в единицах каждого поля. Допуск относится только
к сравнению вычислений. Итоги сохраняются примерно каждые 5 с, при уходе
с экрана и при подтверждении изменений. Аварийное завершение может потерять
последний несохранённый интервал. Подробности модели в [FUEL_COMPUTER.md](../docs/FUEL_COMPUTER.md).

## Журнал LIVE и экспорт JSONL

1. Нажми **Подключить авто**. Приборка начинает локальную запись
   LIVE-сеанса. В неё попадают обновления примерно раз в секунду и
   события между ними: состояние ЭБУ и BLE, изменение поколения
   соединения, пропуски и восстановление покрытия топлива.
2. После остановки открой **Топливный компьютер → Настройки → Журнал LIVE**
   и выбери поездку по времени начала. Журнал доступен офлайн из SIM.
   Все части одной записи открываются одним отчётом.
3. Открытие журнала, переход в SIM, сон или уход с приборки заканчивает
   текущую запись. Если приборка осталась в LIVE, после возврата она
   автоматически начинает новый сеанс.
   Выключение зажигания при открытой приборке оставляет запись текущего сеанса
   активной. Проверь это на припаркованной машине, когда адаптер подключён.

В отчёте есть записанный пробег, оценка израсходованного топлива, средний
расход по участкам с совместным учётом скорости и топлива и доля времени
с таким учётом. Итоги складываются из совместимых соседних измерений
внутри записи. Пропуски данных, смена соединения и сбросы счётчиков
могут сократить рассчитанные итоги.

Ползунок перемещает курсор по всем графикам. Нажатие на событие связи или
Check Engine ставит курсор на его время. Графики топлива и среднего
расхода с момента сброса показывают сохранённые накопительные значения.
Для длинных записей графики прореживаются до 1024 точек на показатель.
Отчёт показывает до 256 событий с приоритетом Check Engine
и указывает, сколько событий пропущено.

Для выгрузки нажми **Экспорт JSONL**, выбери часть и сохрани через
системное окно Android, например в **Загрузки**. Время в имени файла
указано в UTC.

Кнопка **Удалить пустые** находит записи, в которых остались только
попытки запуска и поиска адаптера. Приборка показывает число таких
записей и удаляет их с планшета после подтверждения. Записи с
показаниями, включая нулевые, событиями Check Engine или повреждёнными
строками сохраняются.

В файле есть считанные RPM, MAP, IAT, скорость, температура ОЖ и напряжение
адаптера с признаком доступности и возрастом. Мгновенный расход и итоги
помечены как оценка. Время событий относительно начала сеанса записано в
`elapsed_ms`, UTC-время — в `at`. VIN, адрес адаптера, местоположение и
сырые ответы ЭБУ в журнал не попадают.

Формат `lanos.android.trip` версии 1 состоит из строк `session_start`,
`sample`, `event` и `session_end`. Длинный сеанс разбивается на части с
общим `session_id` и последовательным `part`. Хранятся последние 24 части,
до 2 МиБ каждая. После аварийной остановки у файла может отсутствовать
`session_end`, а последние буферизованные строки могут потеряться. При
экспорте неполная последняя строка пропускается. Экспорт рассчитан на разбор
Android-записей, у старого Go-инструмента `fuelanalyze` другой формат входа.

## Check Engine и коды неисправностей

В режиме LIVE верхняя строка показывает **Check Engine · Горит**,
**Не горит** или **Нет данных**. Число DTC берётся из свежего ответа
`0101` ЭБУ. Значение исчезает при разрыве связи или через 15 секунд
без свежего ответа. **Нет данных** означает, что состояние лампы неизвестно.

Нажми на индикатор **Check Engine**, чтобы открыть ответы ЭБУ для текущего
включения лампы. Сохранённые и ожидающие коды показаны отдельно, со временем
ответа и расшифровкой. Справочник доступен офлайн. Если в нём нет кода,
приборка показывает сам код и сообщает об этом. Успешный пустой список,
ожидание и ошибка чтения имеют разные сообщения.

Для сохранённой записи открой **Журнал LIVE → поездка → События связи и
Check Engine** и нажми на событие включения лампы или сохранённых ответов.
Под событием откроются записанные коды. Расшифровка описывает наблюдение ЭБУ.
Для определения причины могут потребоваться дополнительные измерения.

Когда приборка видит MIL ON, она один раз ставит в очередь чтение
сохранённых (`03`) и ожидающих (`07`) кодов. Это происходит и если лампа
уже горела к началу сеанса или была обнаружена включённой после разрыва.
В этих случаях журнал сохраняет отдельные признаки `MIL_ALREADY_ON`
и `MIL_ON_AFTER_GAP`. Пустой ответ каждого режима сохраняется как
`available` с пустым списком, а отсутствие ответа имеет отдельный статус.

В экспортируемом **Журнале LIVE** `mil_transition` с `incident_id`
указывает на интервал до 60 секунд до события и до 30 секунд после него.
Показания хранятся в обычных строках `sample` того же сеанса, в том числе
по обе стороны границы двух частей. `mil_context_finished` показывает,
сколько точек попало в окно и было ли оно неполным. Результаты `03` и `07`
записываются раздельно как `mil_dtc` с тем же идентификатором. При раннем
уходе с экрана окно закрывается с отметкой о неполноте.

Для первой проверки на припаркованном Lanos подключи LIVE и сравни
индикатор со штатной лампой после включения зажигания и при обычном
запуске двигателя. Если MIL горит, оставь приборку открытой хотя бы
30 секунд и экспортируй JSONL. Для проверки не нужно провоцировать
неисправность или очищать коды.
