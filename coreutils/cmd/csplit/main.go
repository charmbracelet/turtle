// Command csplit is a thin wrapper around the csplit core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/csplit"
)

func main() {
	cmdutil.Main(csplit.New)
}
