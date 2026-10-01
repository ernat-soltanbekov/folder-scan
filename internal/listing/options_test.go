package listing

import (
	"strings"
	"testing"
)

func TestOptionFormsAndLiteralPaths(t *testing.T) {
	a, paths, err := Parse([]string{"-alRrt", "one", "-n", "-1", "-S", "--", "-l", "-"})
	if err != nil || !a.All || !a.Recursive || !a.Reverse || a.Long != !darwin || !a.Numeric || a.Sort != "size" || strings.Join(paths, ",") != "one,-l,-" {
		t.Fatal(a, paths, err)
	}
	b, paths, err := Parse([]string{"-l", "-a", "-t"})
	if err != nil || !b.Long || !b.All || b.Sort != "time" || strings.Join(paths, ",") != "." {
		t.Fatal(b, paths, err)
	}
	_, paths, err = Parse([]string{"-"})
	if err != nil || len(paths) != 1 || paths[0] != "-" {
		t.Fatal("dash operand")
	}
	b, _, err = Parse([]string{"-LHPL", "--color=always", "--no-profile"})
	if err != nil || b.Links != "all" || b.Color != "always" || !b.NoProfile {
		t.Fatal(b, err)
	}
	for _, bad := range []string{"-z", "--unknown", "--color=sometimes", "--color="} {
		if _, _, err := Parse([]string{bad}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func FuzzOptions(f *testing.F) {
	for _, arg := range []string{"-la", "--", "-", "--color=always", "-Rrt"} {
		f.Add(arg)
	}
	f.Fuzz(func(t *testing.T, arg string) {
		o, paths, err := Parse([]string{arg})
		if err != nil {
			return
		}
		if len(paths) == 0 || o.Sort == "" || o.Links == "" {
			t.Fatal("invalid parsed state")
		}
	})
}
