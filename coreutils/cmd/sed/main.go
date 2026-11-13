// Command sed is a thin wrapper around the sed core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/sed"
)

func main() {
	cmdutil.Main(sed.New)
}
