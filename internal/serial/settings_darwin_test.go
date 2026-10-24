package serial

import (
	"syscall"
	"testing"
)

func TestRawSettings(t *testing.T) {
	t.Parallel()
	for _, baud := range []int{9600, 38400} {
		settings := rawSettings(baud)
		if settings.Ispeed != uint64(baud) || settings.Ospeed != uint64(baud) || settings.Lflag != 0 || settings.Oflag != 0 || settings.Iflag != 0 || settings.Cflag != syscall.CS8|syscall.CREAD|syscall.CLOCAL || settings.Cc[syscall.VMIN] != 1 || settings.Cc[syscall.VTIME] != 0 {
			t.Fatalf("not raw 8N1: %+v", settings)
		}
	}
}
