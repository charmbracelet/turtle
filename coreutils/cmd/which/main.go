// Command which is a thin wrapper around the which core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/which"
)

func main() {
	cmdutil.Main(which.New)
}
