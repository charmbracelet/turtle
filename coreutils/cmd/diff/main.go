// Command diff is a thin wrapper around the diff core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/diff"
)

func main() {
	cmdutil.Main(diff.New)
}
