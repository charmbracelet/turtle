// Command gzcat is a thin wrapper around the gzcat core utility.
package main

import (
	"charm.land/snail/coreutils/gzcat"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(gzcat.New)
}
