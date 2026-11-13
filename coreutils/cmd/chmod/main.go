// Command chmod is a thin wrapper around the chmod core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/chmod"
)

func main() {
	cmdutil.Main(chmod.New)
}
