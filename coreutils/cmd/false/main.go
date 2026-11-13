// Command false is a thin wrapper around the false core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/false"
)

func main() {
	cmdutil.Main(false.New)
}
