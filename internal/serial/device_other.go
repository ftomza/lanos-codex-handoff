//go:build !darwin

package serial

import "fmt"

func platformOpen(string, int) (int, error) {
	return -1, fmt.Errorf("real serial access is implemented for macOS only; offline tests work on this host")
}
func platformIO(int, []byte, bool) (int, error) {
	return 0, fmt.Errorf("serial access is unavailable on this host")
}
func platformClose(int) error { return nil }
func retryable(error) bool    { return false }
