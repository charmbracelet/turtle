// Command ls is a thin wrapper around the ls core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/ls"
)

func main() {
	cmdutil.Main(ls.New)
}
