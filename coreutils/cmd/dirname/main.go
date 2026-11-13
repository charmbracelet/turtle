// Command dirname is a thin wrapper around the dirname core utility.
package main

import (
	"charm.land/turtle/coreutils/dirname"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(dirname.New)
}
