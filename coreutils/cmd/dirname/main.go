// Command dirname is a thin wrapper around the dirname core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/dirname"
)

func main() {
	cmdutil.Main(dirname.New)
}
