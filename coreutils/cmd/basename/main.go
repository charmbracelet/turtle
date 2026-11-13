// Command basename is a thin wrapper around the basename core utility.
package main

import (
	"charm.land/turtle/coreutils/basename"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(basename.New)
}
