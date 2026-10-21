// Command obdprobe is reserved for read-only diagnostics in M3.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "obdprobe is unavailable until M3; no hardware access was attempted")
	os.Exit(1)
}
