// Command ln is a thin wrapper around the ln core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/ln"
)

func main() {
	cmdutil.Main(ln.New)
}
