// Command sed is a thin wrapper around the sed core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sed"
)

func main() {
	cmdutil.Main(sed.New)
}
