package listing

// Stable iterative merge sort keeps O(n log n) behavior even for already
// sorted or adversarial names, without importing the disallowed sort package.
func (r *Runner) order(entries []Entry) {
	if len(entries) < 2 {
		return
	}
	scratch := make([]Entry, len(entries))
	for width := 1; width < len(entries); width *= 2 {
		for left := 0; left < len(entries); left += 2 * width {
			middle, end := min(left+width, len(entries)), min(left+2*width, len(entries))
			a, b := left, middle
			for out := left; out < end; out++ {
				if a < middle && (b == end || !r.less(entries[b], entries[a])) {
					scratch[out] = entries[a]
					a++
				} else {
					scratch[out] = entries[b]
					b++
				}
			}
		}
		copy(entries, scratch)
	}
}
func (r *Runner) less(a, b Entry) bool {
	if r.Options.Reverse {
		a, b = b, a
	}
	switch r.Options.Sort {
	case "time":
		if !a.Info.ModTime().Equal(b.Info.ModTime()) {
			return a.Info.ModTime().After(b.Info.ModTime())
		}
	case "size":
		if a.Stat.Size != b.Stat.Size {
			return a.Stat.Size > b.Stat.Size
		}
	}
	return a.Name < b.Name
}
