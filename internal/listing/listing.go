// Package listing reads filesystem metadata directly; it never launches ls.
package listing

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ernat-soltanbekov/folder-scan/internal/ageprofile"
)

type Writer interface{ Write([]byte) (int, error) }

type Metadata struct {
	Links, Inode, Device, RawDevice uint64
	UID, GID                        uint32
	Size, Blocks                    int64
}
type Entry struct {
	Name, Path, Target   string
	Info                 os.FileInfo
	Stat                 Metadata
	Marker, Owner, Group string
}
type ancestor struct {
	device, inode uint64
	parent        *ancestor
}
type directory struct {
	path    string
	header  bool
	parents *ancestor
}

// Runner owns per-invocation caches and a fixed clock. Separate invocations
// share no mutable state, including account-name lookups and output buffers.
type Runner struct {
	Options       Options
	Now           time.Time
	Out, Err      Writer
	Terminal      bool
	buffer        strings.Builder
	writeErr      error
	status        int
	users, groups map[uint32]string
	printed       bool
}

func (r *Runner) Run(paths []string) int {
	r.status, r.printed, r.writeErr = 0, false, nil
	r.buffer.Reset()
	if r.Now.IsZero() {
		r.Now = time.Now()
	}
	r.users = make(map[uint32]string)
	r.groups = make(map[uint32]string)
	var files, dirs []Entry
	for _, path := range paths {
		entry, err := r.inspect(path, path, true)
		if err != nil {
			r.report(path, err, argumentErrorStatus)
			continue
		}
		if entry.Info.IsDir() && !r.Options.Directory {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	r.order(files)
	r.order(dirs)
	if len(files) > 0 {
		r.display(files, false)
		r.printed = true
	}
	for _, entry := range dirs {
		if r.writeErr != nil {
			break
		}
		r.walk(directory{path: entry.Path, header: len(paths) > 1 || (r.Options.Recursive && !darwin)})
	}
	r.flush()
	if r.writeErr != nil {
		fmt.Fprintln(r.Err, "folder-scan: cannot write output")
		return 1
	}
	return r.status
}

func (r *Runner) inspect(path, name string, operand bool) (Entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Entry{}, err
	}
	follow := r.Options.Links == "all" || (operand && r.Options.Links == "command")
	if info.Mode()&os.ModeSymlink != 0 && operand && r.Options.Links == "default" && !r.Options.Long && !r.Options.Directory && !r.Options.Classify {
		if target, e := os.Stat(path); e == nil && target.IsDir() {
			info = target
		}
	}
	if info.Mode()&os.ModeSymlink != 0 && follow {
		info, err = os.Stat(path)
		if err != nil {
			return Entry{}, err
		}
	}
	entry := Entry{Name: name, Path: path, Info: info, Stat: metadata(info)}
	if r.Options.Long {
		if info.Mode()&os.ModeSymlink != 0 {
			entry.Target, err = os.Readlink(path)
			if err != nil {
				return Entry{}, err
			}
		}
		entry.Marker = marker(path, info)
		entry.Owner = r.account(entry.Stat.UID, false)
		entry.Group = r.account(entry.Stat.GID, true)
	}
	return entry, nil
}

func (r *Runner) account(id uint32, group bool) string {
	number := strconv.FormatUint(uint64(id), 10)
	if r.Options.Numeric {
		return number
	}
	cache := r.users
	if group {
		cache = r.groups
	}
	if name, ok := cache[id]; ok {
		return name
	}
	name := number
	if group {
		if entry, err := user.LookupGroupId(number); err == nil {
			name = entry.Name
		}
	} else {
		if entry, err := user.LookupId(number); err == nil {
			name = entry.Username
		}
	}
	cache[id] = name
	return name
}

// An explicit work stack avoids call-stack growth and holds no directory
// descriptor while descending. Ancestry checks stop cycles only on the current
// path, so two independent aliases of a directory can still both be listed.
func (r *Runner) walk(root directory) {
	pending := []directory{root}
	for len(pending) > 0 && r.writeErr == nil {
		task := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		entries, stat, err := r.read(task.path, task.parents != nil && r.Options.Links != "all")
		if err != nil {
			r.report(task.path, err, 1)
			continue
		}
		cycle := false
		for parent := task.parents; parent != nil; parent = parent.parent {
			if parent.device == stat.Device && parent.inode == stat.Inode {
				cycle = true
				break
			}
		}
		if cycle {
			r.report(task.path, errors.New("recursive directory loop"), 1)
			continue
		}
		r.order(entries)
		if r.printed {
			r.emit("\n")
		}
		if task.header {
			r.emit(task.path + ":\n")
		}
		r.display(entries, true)
		r.printed = true
		if !r.Options.NoProfile {
			counts := make(map[string]int, 4)
			for _, entry := range entries {
				if entry.Name != "." && entry.Name != ".." {
					counts[ageprofile.LabelAt(entry.Info.ModTime(), r.Now)]++
				}
			}
			if line := ageprofile.Format(counts); line != "" {
				r.emit(line + "\n")
			}
		}
		if !r.Options.Recursive {
			continue
		}
		parents := &ancestor{stat.Device, stat.Inode, task.parents}
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if entry.Info.IsDir() && entry.Name != "." && entry.Name != ".." {
				pending = append(pending, directory{entry.Path, true, parents})
			}
		}
	}
}

func join(directory, name string) string {
	if strings.HasSuffix(directory, "/") {
		return directory + name
	}
	return directory + "/" + name
}

func (r *Runner) read(path string, noFollow bool) ([]Entry, Metadata, error) {
	// O_DIRECTORY and O_NONBLOCK prevent a concurrently replaced directory
	// from turning into a blocking FIFO open. f.Stat identifies the directory
	// actually opened, avoiding a separate path-stat race in cycle detection.
	flags := syscall.O_RDONLY | syscall.O_DIRECTORY | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	if noFollow {
		flags |= syscall.O_NOFOLLOW
	}
	fd, err := syscall.Open(path, flags, 0)
	if err != nil {
		return nil, Metadata{}, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Metadata{}, err
	}
	stat := metadata(info)
	names, readErr := f.Readdirnames(-1)
	closeErr := f.Close()
	if readErr != nil {
		return nil, Metadata{}, readErr
	}
	if closeErr != nil {
		return nil, Metadata{}, closeErr
	}
	if r.Options.All {
		names = append(names, ".", "..")
	}
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, ".") && !r.Options.All && !r.Options.AlmostAll {
			continue
		}
		entry, err := r.inspect(join(path, name), name, false)
		if err != nil {
			r.report(join(path, name), err, 1)
			continue
		}
		entries = append(entries, entry)
	}
	return entries, stat, nil
}

func (r *Runner) report(path string, err error, status int) {
	var problem *os.PathError
	if errors.As(err, &problem) {
		err = problem.Err
	}
	fmt.Fprintf(r.Err, "folder-scan: %s: %s\n", path, err)
	if status > r.status {
		r.status = status
	}
}

// Output is batched with only the allowed packages. Short writes are retried;
// a zero-length successful write is rejected instead of creating an endless loop.
func (r *Runner) emit(text string) {
	if r.writeErr != nil {
		return
	}
	r.buffer.WriteString(text)
	if r.buffer.Len() >= 65536 {
		r.flush()
	}
}
func (r *Runner) flush() {
	if r.writeErr != nil || r.buffer.Len() == 0 {
		return
	}
	data := []byte(r.buffer.String())
	r.buffer.Reset()
	for len(data) > 0 {
		n, err := r.Out.Write(data)
		if err != nil {
			r.writeErr = err
			return
		}
		if n <= 0 || n > len(data) {
			r.writeErr = errors.New("invalid output write")
			return
		}
		data = data[n:]
	}
}
