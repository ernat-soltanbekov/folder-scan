package listing

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

const darwin = true
const defaultBlockSize int64 = 512
const argumentErrorStatus = 1

func platformG(o *Options) { o.Color = "auto" }
func metadata(info os.FileInfo) Metadata {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Metadata{Links: 1}
	}
	return Metadata{Links: uint64(s.Nlink), UID: s.Uid, GID: s.Gid, Size: s.Size, Blocks: s.Blocks, Inode: s.Ino, Device: uint64(s.Dev), RawDevice: uint64(uint32(s.Rdev))}
}
func deviceSize(raw uint64, majorWidth, minorWidth int) string {
	if raw == 0 {
		return "0"
	}
	return fmt.Sprintf("%#x", raw)
}
func marker(path string, info os.FileInfo) string {
	// flistxattr with a nil buffer returns only the required name-list length.
	// Scalar descriptor arguments avoid unsafe, cgo and extra Go packages.
	// Never open devices, FIFOs or sockets: opening a clone device can change
	// its state even with a metadata-only descriptor.
	if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return " "
	}
	flags := syscall.O_EVTONLY | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	if info.Mode()&os.ModeSymlink != 0 {
		flags |= syscall.O_SYMLINK
	}
	fd, err := syscall.Open(path, flags, 0)
	if err != nil {
		return " "
	}
	defer syscall.Close(fd)
	size, _, errno := syscall.Syscall6(syscall.SYS_FLISTXATTR, uintptr(fd), 0, 0, 0, 0, 0)
	if errno == 0 && size > 0 {
		return "@"
	}
	return " " // ACL '+' is outside the permitted high-level Darwin API.
}
func recent(stamp, now time.Time) bool {
	// Native BSD formatting compares whole Unix seconds with a 182-day window.
	const halfYearSeconds = (365 / 2) * 86400
	return stamp.Unix() > now.Unix()-halfYearSeconds && stamp.Unix() <= now.Unix()
}

func deviceParts(raw uint64) (uint64, uint64) { return raw >> 24, raw & 0xffffff }
