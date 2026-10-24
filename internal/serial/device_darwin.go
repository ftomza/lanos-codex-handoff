package serial

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

func rawSettings(baud int) syscall.Termios {
	settings := syscall.Termios{Cflag: syscall.CS8 | syscall.CREAD | syscall.CLOCAL, Ispeed: uint64(baud), Ospeed: uint64(baud)}
	settings.Cc[syscall.VMIN] = 1
	return settings
}

func platformOpen(path string, baud int) (int, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return -1, err
	}
	if err := configure(fd, baud); err != nil {
		return -1, errors.Join(err, syscall.Close(fd))
	}
	syscall.CloseOnExec(fd)
	return fd, nil
}

func configure(fd, baud int) error {
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFCHR {
		return fmt.Errorf("serial path is not a character device")
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCEXCL, 0); errno != 0 {
		return errno
	}
	settings := rawSettings(baud)
	// TIOCSETA applies raw settings without flushing any pending response bytes.
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCSETA, uintptr(unsafe.Pointer(&settings))); errno != 0 {
		return errno
	}
	return nil
}

func platformIO(fd int, b []byte, write bool) (int, error) {
	if write {
		return syscall.Write(fd, b)
	}
	return syscall.Read(fd, b)
}
func platformClose(fd int) error { return syscall.Close(fd) }
func retryable(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR)
}
