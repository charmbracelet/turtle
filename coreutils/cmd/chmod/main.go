// Command chmod is a thin wrapper around the chmod core utility.
package main

import (
	"charm.land/snail/coreutils/chmod"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(chmod.New)
}
