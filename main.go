package main

import (
	"fmt"
	"os"

	"github.com/ernat-soltanbekov/folder-scan/internal/listing"
)

func main() {
	options, paths, err := listing.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "folder-scan:", err)
		os.Exit(2)
	}
	if options.Help {
		fmt.Print(listing.Help)
		return
	}
	if options.Version {
		fmt.Println("folder-scan 1.0.0")
		return
	}
	terminal := false
	if info, err := os.Stdout.Stat(); err == nil {
		terminal = info.Mode()&os.ModeCharDevice != 0
	}
	runner := listing.Runner{Options: options, Out: os.Stdout, Err: os.Stderr, Terminal: terminal}
	os.Exit(runner.Run(paths))
}
