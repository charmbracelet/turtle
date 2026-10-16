// Command rmdir is a thin wrapper around the rmdir core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/rmdir"
)

func main() {
	cmdutil.Main(rmdir.New)
}
