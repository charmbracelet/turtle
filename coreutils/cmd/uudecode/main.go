// Command uudecode is a thin wrapper around the uudecode core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/uudecode"
)

func main() {
	cmdutil.Main(uudecode.New)
}
