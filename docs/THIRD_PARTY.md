# Native BLE dependencies

The user approved tinygo.org/x/bluetooth v0.16.0 and its required transitive
dependencies. Go standard library has no CoreBluetooth/GATT client API.
go.mod/go.sum pin the full graph; no Python runtime is used by obdprobe.

The native macOS helper links these external modules (verified with
`go list -tags nativeble -deps ./cmd/obdprobe-ble`):

| Module | Version | License copy |
| --- | --- | --- |
| tinygo.org/x/bluetooth | v0.16.0 | [BSD-3-Clause](licenses/tinygo-bluetooth.txt) |
| github.com/tinygo-org/cbgo | v0.0.4 | [Apache-2.0](licenses/cbgo.txt) |
| github.com/sirupsen/logrus | v1.9.3 | [MIT](licenses/logrus.txt) |
| golang.org/x/sys | v0.11.0 | [BSD-3-Clause](licenses/golang-sys.txt) |

Keep these notices/license copies with redistributed native binaries. Other
indirect requirements in go.mod support the upstream library's other targets;
their presence does not claim those platforms or Android have been tested here.
