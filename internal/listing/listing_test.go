package listing

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func execute(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	o, paths, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr strings.Builder
	r := Runner{Options: o, Now: time.Now(), Out: &out, Err: &stderr}
	code := r.Run(paths)
	return out.String(), stderr.String(), code
}
func file(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("data"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryProfilesAndEmptyCases(t *testing.T) {
	root := t.TempDir()
	for _, flags := range [][]string{{root}, {"-a", root}, {"-l", root}} {
		out, stderr, code := execute(t, flags...)
		if code != 0 || stderr != "" || strings.Contains(out, "Age profile:") {
			t.Fatal(out, stderr, code)
		}
	}
	file(t, join(root, "fresh"), 0600)
	file(t, join(root, "old"), 0600)
	file(t, join(root, ".hidden"), 0600)
	old := time.Now().Add(-400 * 24 * time.Hour)
	if err := os.Chtimes(join(root, "old"), old, old); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := execute(t, root)
	if code != 0 || stderr != "" || out != "fresh\nold\nAge profile: fresh 1 | archived 1\n" {
		t.Fatal(out, stderr, code)
	}
	out, _, _ = execute(t, "-a", root)
	if !strings.Contains(out, "Age profile: fresh 2 | archived 1") {
		t.Fatal("dot entries counted, or hidden entry omitted", out)
	}
	out, _, _ = execute(t, join(root, "old"))
	if strings.Contains(out, "Age profile:") {
		t.Fatal("profile for file operand")
	}
	out, _, _ = execute(t, "-d", root)
	if strings.Contains(out, "Age profile:") {
		t.Fatal("profile for unlisted directory")
	}
}

func TestRecursionOrderAndProfiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.Mkdir(join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		file(t, join(join(root, name), "file"), 0600)
	}
	if err := os.Symlink("alpha", join(root, "link")); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := execute(t, "-Rr", root)
	if code != 0 || stderr != "" || strings.Count(out, "Age profile:") != 3 {
		t.Fatal(out, stderr, code)
	}
	if strings.Index(out, join(root, "beta")+":") > strings.Index(out, join(root, "alpha")+":") {
		t.Fatal("reverse recursive order")
	}
	if strings.Contains(out, join(root, "link")+":") {
		t.Fatal("followed a physical symlink")
	}
	if err := os.Symlink("..", join(join(root, "alpha"), "loop")); err != nil {
		t.Fatal(err)
	}
	out, stderr, code = execute(t, "-RL", root)
	if code == 0 || !strings.Contains(stderr, "recursive directory loop") {
		t.Fatal(out, stderr, code)
	}
	if !strings.Contains(out, join(root, "link")+":") {
		t.Fatal("independent alias was incorrectly suppressed")
	}
}

func TestLinkOperandsAndTrailingSlash(t *testing.T) {
	root := t.TempDir()
	file(t, join(root, "target"), 0600)
	if err := os.Mkdir(join(root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	file(t, join(root, "dir/child"), 0600)
	for _, pair := range [][2]string{{"file-link", "target"}, {"dir-link", "dir"}, {"broken", "missing"}} {
		if err := os.Symlink(pair[1], join(root, pair[0])); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []string{"file-link", "dir-link", "broken"} {
		out, stderr, code := execute(t, "-l", join(root, link))
		if code != 0 || stderr != "" || !strings.Contains(out, " -> ") {
			t.Fatal(out, stderr, code)
		}
	}
	out, _, code := execute(t, "-l", join(root, "dir-link/"))
	if code != 0 || strings.Contains(out, " -> ") || !strings.Contains(out, "child") {
		t.Fatal(out, code)
	}
	out, stderr, code := execute(t, "-l", join(root, "file-link/"))
	if code == 0 || out != "" || stderr == "" {
		t.Fatal(out, stderr, code)
	}
	out, _, code = execute(t, join(root, "dir-link"))
	if code != 0 || !strings.Contains(out, "child") {
		t.Fatal(out, code)
	}
	out, _, code = execute(t, "-F", join(root, "dir-link"))
	if code != 0 || !strings.Contains(out, "dir-link@") {
		t.Fatal(out, code)
	}
}

func TestMissingOperandDoesNotHideValidOperand(t *testing.T) {
	root := t.TempDir()
	path := join(root, "exists")
	file(t, path, 0600)
	out, stderr, code := execute(t, join(root, "missing"), path)
	if code == 0 || out != path+"\n" || !strings.Contains(stderr, "missing") {
		t.Fatal(out, stderr, code)
	}
}

func TestDirectoryOpenRejectsFIFOAndReplacedSymlink(t *testing.T) {
	root := t.TempDir()
	pipe := join(root, "pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	r := Runner{}
	if _, _, err := r.read(pipe, false); err == nil {
		t.Fatal("FIFO opened as a directory")
	}
	link := join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.read(link, true); err == nil {
		t.Fatal("physical traversal followed a replaced symlink")
	}
	var stderr strings.Builder
	r.Err = &stderr
	_, got, err := r.read(link, false)
	want, statErr := os.Stat(root)
	if err != nil || statErr != nil || got.Inode != metadata(want).Inode {
		t.Fatal("identity does not belong to opened directory", err, statErr)
	}
}

func TestPermissionModes(t *testing.T) {
	for _, test := range []struct {
		mode os.FileMode
		want string
	}{
		{0644, "-rw-r--r--"}, {os.ModeDir | 0755, "drwxr-xr-x"}, {os.ModeSymlink | 0777, "lrwxrwxrwx"},
		{os.ModeNamedPipe | 0600, "prw-------"}, {os.ModeSocket | 0600, "srw-------"},
		{os.ModeDevice | os.ModeCharDevice | 0600, "crw-------"}, {os.ModeDevice | 0600, "brw-------"},
		{os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0777, "-rwsrwsrwt"}, {os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0666, "-rwSrwSrwT"},
	} {
		if got := permissions(test.mode); got != test.want {
			t.Fatal(got, test.want)
		}
	}
}

func TestNativeDateWindow(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if !recent(now, now) || !recent(now.Add(-24*time.Hour), now) {
		t.Fatal("recent date uses year")
	}
	if recent(now.Add(time.Hour), now) || recent(now.Add(-400*24*time.Hour), now) {
		t.Fatal("future or ancient date uses clock")
	}
	if darwin {
		boundary := now.Add(-182 * 24 * time.Hour)
		if recent(boundary, now) || !recent(boundary.Add(time.Second), now) {
			t.Fatal("BSD whole-second date boundary")
		}
	} else {
		boundary := now.Add(-time.Duration(31556952/2) * time.Second)
		if recent(boundary, now) || !recent(boundary.Add(time.Nanosecond), now) {
			t.Fatal("GNU date boundary")
		}
	}
}

type brokenWriter struct{ zero bool }

func (w brokenWriter) Write([]byte) (int, error) {
	if w.zero {
		return 0, nil
	}
	return 0, errors.New("broken pipe")
}

type shortWriter struct{ data strings.Builder }

func (w *shortWriter) Write(data []byte) (int, error) {
	if len(data) > 3 {
		data = data[:3]
	}
	return w.data.Write(data)
}

func TestOutputFailuresAndShortWrites(t *testing.T) {
	root := t.TempDir()
	file(t, join(root, "test"), 0600)
	for _, writer := range []Writer{brokenWriter{}, brokenWriter{true}} {
		var stderr strings.Builder
		r := Runner{Options: Options{}, Out: writer, Err: &stderr}
		if r.Run([]string{root}) == 0 || !strings.Contains(stderr.String(), "cannot write") {
			t.Fatal("output error ignored")
		}
	}
	var out shortWriter
	var stderr strings.Builder
	r := Runner{Out: &out, Err: &stderr}
	if r.Run([]string{root}) != 0 || out.data.String() != "test\nAge profile: fresh 1\n" {
		t.Fatal(out.data.String(), stderr.String())
	}
}

func TestColorAndIndicators(t *testing.T) {
	root := t.TempDir()
	file(t, join(root, "run"), 0700)
	if err := os.Mkdir(join(root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	out, _, code := execute(t, "--color=always", "-F", root)
	if code != 0 || !strings.Contains(out, "\x1b[34mdir\x1b[0m/") || !strings.Contains(out, "\x1b[32mrun\x1b[0m*") {
		t.Fatal(out, code)
	}
	out, _, _ = execute(t, "--color=auto", root)
	if strings.Contains(out, "\x1b") {
		t.Fatal("auto colored a pipe")
	}
}

func TestLargeNameSortAndTimeTies(t *testing.T) {
	random := rand.New(rand.NewSource(2008))
	entries := make([]Entry, 10000)
	for i, value := range random.Perm(len(entries)) {
		entries[i].Name = fmt.Sprintf("%05d", value)
	}
	r := Runner{Options: Options{Sort: "name"}}
	r.order(entries)
	for i, entry := range entries {
		if entry.Name != fmt.Sprintf("%05d", i) {
			t.Fatal("merge sort order", i)
		}
	}
	r.Options.Reverse = true
	r.order(entries)
	for i, entry := range entries {
		if entry.Name != fmt.Sprintf("%05d", len(entries)-1-i) {
			t.Fatal("reverse order", i)
		}
	}
	root := t.TempDir()
	stamp := time.Now().Add(-time.Hour)
	for _, name := range []string{"b", "a", "c"} {
		path := join(root, name)
		file(t, path, 0600)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	out, _, _ := execute(t, "-tr", "--no-profile", root)
	if out != "c\nb\na\n" {
		t.Fatal("time tie order", out)
	}
}

func BenchmarkLargeDirectorySort(b *testing.B) {
	entries := make([]Entry, 10000)
	for i := range entries {
		entries[i].Name = fmt.Sprintf("%05d", len(entries)-i)
	}
	r := Runner{Options: Options{Sort: "name"}}
	b.ResetTimer()
	for range b.N {
		r.order(entries)
	}
}
