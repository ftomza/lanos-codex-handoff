# Native BLE obdprobe Implementation Plan

> Execute inline, task by task, using writing-plans, executing-plans and TDD.

**Goal:** Run the existing read-only Go probe over native macOS BLE.

**Architecture:** Keep ELM/probe in obdprobe; isolate CoreBluetooth initialization
and blocking APIs in a native Go child. Communicate bounded byte events over pipes.

**Tech stack:** Go 1.27, stdlib, tinygo.org/x/bluetooth v0.16.0, macOS CGO.

## 1. Transport-aware runner and report

Files: internal/probe/runner.go, scan.go, report.go, ble_test.go;
cmd/obdprobe/output.go and output_test.go.

- [x] Write a failing test using fixtureAdapter: Config{Transport: "ble"} runs
  ATE0/ATL0/ATI/ATSP0 without ATZ, exports ble_gatt/elm327_ble and BLE profile.
- [x] Test that BLE open gets ConnectTimeout rather than the ELM Timeout and
  that serial initialization/metadata stay unchanged.
- [x] Add Config fields Transport, BLEName, BLEHelper, ConnectTimeout and BLE
  report metadata. Default empty Transport to serial; reject unknown transports.
- [x] Select initialization and open deadline by transport. Keep recovery,
  discovery and allowlisting in existing code. Print adapter ATRV in human output.
- [x] Run go test ./internal/probe ./cmd/obdprobe and confirm regression coverage.

## 2. Bounded native-helper protocol and parent transport

Files: internal/ble/{protocol,device,process_test}.go.

- [x] Write Go test-child scenarios for ready, notification-before-ack,
  fragmented bytes, EOF, wrong ack ID, overlong message, queue overflow,
  startup cancellation and cleanup. Use os.Executable and -test.run so tests
  need no external helper, Python or actual hardware.
- [x] Define JSON messages with Type, ID, Data ([]byte/base64), Error; maximum
  line 32 KiB and receive buffer 64 KiB. Reject invalid event shapes/unknown IDs.
- [x] Implement Open(ctx, Config), Read(ctx, []byte), Write(ctx, []byte), Close().
  Start with exec.Command, read stdout on one goroutine, synchronize buffer and
  pending ack, preserve bytes; no context stored in fields. Open context only
  bounds readiness. Wait for write ack with matching ID. Close writes close,
  waits a bounded grace then kills/reaps. Wait is called exactly once.
- [x] Run go test -race ./internal/ble and prove no leaked or blocked child.

## 3. Native helper with fake-backend tests

Files: internal/blehelper/{serve,serve_test,native_darwin,native_stub}.go;
cmd/obdprobe-ble/main.go; Makefile.

- [x] Write protocol tests using injected Connection (Write, Close) and Open
  callbacks; verify ready after subscription, write acknowledgements, error /
  EOF / close cleanup and notification ordering. Fake is stdlib only.
- [x] Implement Serve(ctx, name, input, output, opener); bounded scanner, serialized
  output, finite event queue, no ELM parser. Notification overflow fails closed.
- [x] Add nativeble && darwin && cgo implementation: enable adapter, scan exact
  name, stop scan, connect, discover known service/RX/TX, subscribe before ready,
  acknowledged Write, disconnect callback, disable notifications and disconnect.
  Other builds return an explicit native-backend-unavailable error.
- [x] Build helper only with -tags=nativeble. make build-obdprobe creates
  bin/obdprobe and bin/obdprobe-ble beside each other. Ordinary go test ./...
  never imports the CoreBluetooth backend.
- [x] Run fake-backend tests and compile the native helper on macOS.

## 4. CLI transport selection

Files: cmd/obdprobe/{cli,ble_test}.go.

- [x] Test BLE invocation without serial port, rejected mixed flags, invalid
  names/deadlines, serial compatibility, no hardware during help/list/invalid flags.
- [x] Add --transport, --ble-name, --ble-connect-timeout, --ble-helper. Reject
  explicitly mixed flags. Choose ble.Open or serial.Open inside runProbe.
- [x] Ensure missing-helper errors are exported to private reports and suggest
  make build-obdprobe. No fallback or automatic package installation.
- [x] Run go test ./... -count=1; go test -race ./...; make lint; go build ./...;
  make test-web; GOOS=linux GOARCH=amd64 go build ./...; native helper build.

## 5. Evidence and delivery

- [x] Add real engine-on BLE response fixture with local-log provenance and
  verify shared Go decoder against RPM/temperature/0120/ATRV observations.
- [x] Update README, NEXT_CONNECTION, capabilities and STATUS with runtime/build
  requirements, exact command and actual offline/hardware results.
- [x] Confirm current car/ignition state before a bounded physical Go probe;
  preserve raw private output. Physical availability is not required for unit
  tests; if unavailable, explicitly leave Go hardware validation pending.
- [x] Review diff and git diff --check; commit/push only intended files with
  required trailer. Preserve untracked user scripts and ignored private reports.

Review: the native library is never imported by the parent or default test
build. Open context cancellation after ready cannot close the connection.
Timeout recovery remains shared and cannot silently write after incomplete frames.
Dashboard and M4 remain outside this implementation.

Result: native Go baseline completed with 12/12 commands OK; see STATUS.md.
