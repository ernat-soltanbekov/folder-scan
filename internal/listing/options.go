package listing

import (
	"fmt"
	"strings"
)

type Options struct {
	Long, All, AlmostAll, Reverse, Recursive   bool
	Directory, Numeric, Inode, Classify, Slash bool
	NoGroup, Kibi, NoProfile, Help, Version    bool
	Sort                                       string // name, time or size; the last sort option wins
	Links                                      string // default, command, all or physical
	Color                                      string // never, auto or always
}

// Parse accepts both -lat and -l -a -t, interspersed operands, and --.
// A single '-' is a literal filename, never stdin.
func Parse(args []string) (Options, []string, error) {
	o := Options{Sort: "name", Links: "default", Color: "never"}
	var paths []string
	options := true
	for _, arg := range args {
		if options && arg == "--" {
			options = false
			continue
		}
		if !options || arg == "-" || !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch {
			case arg == "--help":
				o.Help = true
			case arg == "--version":
				o.Version = true
			case arg == "--no-profile":
				o.NoProfile = true
			case arg == "--color":
				o.Color = "always"
			case strings.HasPrefix(arg, "--color="):
				o.Color = strings.TrimPrefix(arg, "--color=")
				if o.Color != "always" && o.Color != "auto" && o.Color != "never" {
					return o, nil, fmt.Errorf("invalid color mode %q", o.Color)
				}
			default:
				return o, nil, fmt.Errorf("unrecognized option %q", arg)
			}
			continue
		}
		for _, flag := range arg[1:] {
			switch flag {
			case 'l':
				o.Long = true
			case '1':
				if darwin {
					o.Long = false
				} // GNU ls keeps an earlier -l.
			case 'a':
				o.All = true
			case 'A':
				o.AlmostAll = true
				if !darwin {
					o.All = false
				}
			case 'r':
				o.Reverse = true
			case 't':
				o.Sort = "time"
			case 'S':
				o.Sort = "size"
			case 'R':
				o.Recursive = true
			case 'd':
				o.Directory = true
			case 'n':
				o.Numeric = true
				o.Long = true
			case 'i':
				o.Inode = true
			case 'F':
				o.Classify = true
			case 'p':
				o.Slash = true
			case 'k':
				o.Kibi = true
			case 'L':
				o.Links = "all"
			case 'H':
				o.Links = "command"
			case 'P':
				o.Links = "physical"
			case 'G':
				platformG(&o)
			default:
				return o, nil, fmt.Errorf("invalid option -- %c", flag)
			}
		}
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return o, paths, nil
}

const Help = `Usage: folder-scan [options] [--] [path ...]
List files with a rule-based modification-age profile after each directory.

  -l       long listing            -a       include . and .. and hidden names
  -r       reverse order           -t       newest modification first
  -R       recursive listing       -A       hidden names except . and ..
  -S       largest first           -d       list directories themselves
  -n       numeric owner/group     -i       include inode number
  -F       file-type suffixes      -p       slash after directory names
  -1       one entry per line      -k       totals in 1024-byte blocks
  -L       follow all symlinks     -H       follow command-line symlinks
  -P       do not follow symlinks   -G       native platform meaning
  --color[=always|auto|never]       ANSI colors (default: never)
  --no-profile                     omit the added age summary
  --help                           show this help
  --version                        show version

The last sort option wins. BSD -1 overrides -l; GNU -l takes priority over -1.
Use -- before a dash-leading path.
Profiles count listed entries of all types, excluding . and ... Future = fresh.
`
