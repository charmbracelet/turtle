// Command chmod is a thin wrapper around the chmod core utility.
package main

import (
	"charm.land/turtle/coreutils/chmod"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(chmod.New)
}
