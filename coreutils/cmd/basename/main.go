// Command basename is a thin wrapper around the basename core utility.
package main

import (
	"charm.land/snail/coreutils/basename"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(basename.New)
}
