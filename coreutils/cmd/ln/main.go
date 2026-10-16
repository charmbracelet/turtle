// Command ln is a thin wrapper around the ln core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/ln"
)

func main() {
	cmdutil.Main(ln.New)
}
