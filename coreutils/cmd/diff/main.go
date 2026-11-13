// Command diff is a thin wrapper around the diff core utility.
package main

import (
	"charm.land/turtle/coreutils/diff"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(diff.New)
}
