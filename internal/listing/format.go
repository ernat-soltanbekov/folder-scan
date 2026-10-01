package listing

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type widths struct {
	links, owner, group, size, inode, major, minor int
	marker                                         bool
}

func (r *Runner) display(entries []Entry, directory bool, widthOperands []Entry) {
	// The common recursive listing needs no column measurements or formatting.
	if !r.Options.Long && !r.Options.Inode {
		for _, entry := range entries {
			r.emit(r.name(entry) + "\n")
		}
		return
	}
	w := widths{}
	var blocks int64
	measured := entries
	// GNU ls measures all command-line operands before separating files from
	// directory listings. BSD measures the displayed file group on its own.
	if len(widthOperands) > 0 {
		measured = append(append([]Entry(nil), entries...), widthOperands...)
	}
	for _, entry := range measured {
		w.links = max(w.links, len(strconv.FormatUint(entry.Stat.Links, 10)))
		w.owner = max(w.owner, len([]rune(entry.Owner)))
		w.group = max(w.group, len([]rune(entry.Group)))
		w.inode = max(w.inode, len(strconv.FormatUint(entry.Stat.Inode, 10)))
		w.size = max(w.size, len(strconv.FormatInt(entry.Stat.Size, 10)))
		if entry.Marker != " " && entry.Marker != "" {
			w.marker = true
		}
		if entry.Info.Mode()&os.ModeDevice != 0 {
			if darwin {
				w.size = max(w.size, len(deviceSize(entry.Stat.RawDevice, 0, 0)))
			} else {
				major, minor := deviceParts(entry.Stat.RawDevice)
				w.major = max(w.major, len(strconv.FormatUint(major, 10)))
				w.minor = max(w.minor, len(strconv.FormatUint(minor, 10)))
			}
		}
		blocks += entry.Stat.Blocks
	}
	if !darwin && w.major > 0 {
		w.size = max(w.size, w.major+2+w.minor)
	}
	if r.Options.Long && directory {
		blockSize := defaultBlockSize
		if r.Options.Kibi {
			blockSize = 1024
		}
		r.emit(fmt.Sprintf("total %d\n", (blocks*512+blockSize-1)/blockSize))
	}
	for _, entry := range entries {
		prefix := ""
		if r.Options.Inode {
			prefix = fmt.Sprintf("%*d ", w.inode, entry.Stat.Inode)
		}
		if r.Options.Long {
			prefix += r.long(entry, w)
		}
		r.emit(prefix + r.name(entry) + "\n")
	}
}

func permissions(mode os.FileMode) string {
	text := []byte("----------")
	switch {
	case mode&os.ModeSymlink != 0:
		text[0] = 'l'
	case mode.IsDir():
		text[0] = 'd'
	case mode&os.ModeNamedPipe != 0:
		text[0] = 'p'
	case mode&os.ModeSocket != 0:
		text[0] = 's'
	case mode&os.ModeCharDevice != 0:
		text[0] = 'c'
	case mode&os.ModeDevice != 0:
		text[0] = 'b'
	}
	for i, letter := range []byte("rwxrwxrwx") {
		if mode.Perm()&(1<<uint(8-i)) != 0 {
			text[i+1] = letter
		}
	}
	for _, special := range []struct {
		bit     os.FileMode
		index   int
		on, off byte
	}{{os.ModeSetuid, 3, 's', 'S'}, {os.ModeSetgid, 6, 's', 'S'}, {os.ModeSticky, 9, 't', 'T'}} {
		if mode&special.bit != 0 {
			if text[special.index] == 'x' {
				text[special.index] = special.on
			} else {
				text[special.index] = special.off
			}
		}
	}
	return string(text)
}

func (r *Runner) long(entry Entry, w widths) string {
	mode := permissions(entry.Info.Mode())
	if darwin || w.marker {
		mode += entry.Marker
	}
	sep := " "
	if darwin {
		sep = "  "
	}
	owner, group := fmt.Sprintf("%-*s", w.owner, entry.Owner), fmt.Sprintf("%-*s", w.group, entry.Group)
	if !darwin {
		if r.Options.Numeric || entry.Owner == strconv.FormatUint(uint64(entry.Stat.UID), 10) {
			owner = fmt.Sprintf("%*s", w.owner, entry.Owner)
		}
		if r.Options.Numeric || entry.Group == strconv.FormatUint(uint64(entry.Stat.GID), 10) {
			group = fmt.Sprintf("%*s", w.group, entry.Group)
		}
	}
	accounts := owner + sep
	if !r.Options.NoGroup {
		accounts += group + sep
	}
	size := strconv.FormatInt(entry.Stat.Size, 10)
	if entry.Info.Mode()&os.ModeDevice != 0 {
		size = deviceSize(entry.Stat.RawDevice, w.major, w.minor)
	}
	return fmt.Sprintf("%s %*d %s%*s %s ", mode, w.links, entry.Stat.Links, accounts, w.size, size, date(entry.Info.ModTime(), r.Now))
}

func date(stamp, now time.Time) string {
	stamp = stamp.Local()
	if recent(stamp, now) {
		return stamp.Format("Jan _2 15:04")
	}
	return stamp.Format("Jan _2  2006")
}

func (r *Runner) name(entry Entry) string {
	name := entry.Name
	mode := entry.Info.Mode()
	if r.Options.Color == "always" || (r.Options.Color == "auto" && r.Terminal) {
		color := ""
		switch {
		case mode&os.ModeSymlink != 0:
			color = "36"
		case mode.IsDir():
			color = "34"
		case mode&os.ModeNamedPipe != 0 || mode&os.ModeDevice != 0:
			color = "33"
		case mode&os.ModeSocket != 0:
			color = "35"
		case mode.Perm()&0111 != 0:
			color = "32"
		}
		if color != "" {
			name = "\x1b[" + color + "m" + name + "\x1b[0m"
		}
	}
	if r.Options.Classify {
		switch {
		case mode.IsDir():
			name += "/"
		case mode&os.ModeSymlink != 0:
			name += "@"
		case mode&os.ModeNamedPipe != 0:
			name += "|"
		case mode&os.ModeSocket != 0:
			name += "="
		case mode.IsRegular() && mode.Perm()&0111 != 0:
			name += "*"
		}
	} else if r.Options.Slash && mode.IsDir() {
		name += "/"
	}
	if r.Options.Long && mode&os.ModeSymlink != 0 {
		name += " -> " + entry.Target
	}
	return name
}
