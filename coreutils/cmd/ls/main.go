// Command ls is a thin wrapper around the ls core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/ls"
)

func main() {
	cmdutil.Main(ls.New)
}
