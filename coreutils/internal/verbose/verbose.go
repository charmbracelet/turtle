// Package verbose holds the reporting levels of the utilities which change
// file attributes, selected by the -c and -v options, and the names they
// show the files they walk by in their diagnostics.
package verbose

import (
	"path"
	"strings"
)

// The levels of a run, selected by -c and -v.
const (
	Quiet = iota
	ChangesOnly
	Noisy
)

// Display returns the path to show in diagnostics: the operand as given,
// with any sub-path walked below it appended.
func Display(operand, resolved, walked string) string {
	if walked == resolved {
		return operand
	}
	if rel, ok := strings.CutPrefix(walked, resolved+"/"); ok {
		return path.Join(operand, rel)
	}
	return path.Join(operand, walked)
}
