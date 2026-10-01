package listing

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

const darwin = false
const defaultBlockSize int64 = 1024
const argumentErrorStatus = 2

func platformG(o *Options) { o.NoGroup = true }
func metadata(info os.FileInfo) Metadata {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Metadata{Links: 1}
	}
	return Metadata{Links: uint64(s.Nlink), UID: s.Uid, GID: s.Gid, Size: s.Size, Blocks: s.Blocks, Inode: s.Ino, Device: uint64(s.Dev), RawDevice: uint64(s.Rdev)}
}
func deviceParts(raw uint64) (uint64, uint64) {
	return (raw>>8)&0xfff | (raw>>32)&0xfffff000, raw&0xff | (raw>>12)&0xffffff00
}
func deviceSize(raw uint64, majorWidth, minorWidth int) string {
	major, minor := deviceParts(raw)
	return fmt.Sprintf("%*d, %*d", majorWidth, major, minorWidth, minor)
}
func marker(path string, info os.FileInfo) string {
	if info.Mode()&os.ModeSymlink != 0 {
		return " "
	}
	if size, err := syscall.Getxattr(path, "system.posix_acl_access", nil); err == nil && size > 28 {
		return "+"
	}
	if info.IsDir() {
		if size, err := syscall.Getxattr(path, "system.posix_acl_default", nil); err == nil && size > 0 {
			return "+"
		}
	}
	if size, err := syscall.Getxattr(path, "security.selinux", nil); err == nil && size > 0 {
		return "."
	}
	return " "
}
func recent(stamp, now time.Time) bool {
	// GNU ls uses half the average Gregorian year, allowing no future date.
	const halfYear = time.Duration(31556952/2) * time.Second
	return stamp.After(now.Add(-halfYear)) && !stamp.After(now)
}
