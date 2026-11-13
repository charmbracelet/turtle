// Command basename is a thin wrapper around the basename core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/basename"
)

func main() {
	cmdutil.Main(basename.New)
}
