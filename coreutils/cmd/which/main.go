// Command which is a thin wrapper around the which core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/which"
)

func main() {
	cmdutil.Main(which.New)
}
