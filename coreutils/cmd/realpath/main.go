// Command realpath is a thin wrapper around the realpath core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/realpath"
)

func main() {
	cmdutil.Main(realpath.New)
}
