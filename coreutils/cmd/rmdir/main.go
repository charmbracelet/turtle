// Command rmdir is a thin wrapper around the rmdir core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/rmdir"
)

func main() {
	cmdutil.Main(rmdir.New)
}
