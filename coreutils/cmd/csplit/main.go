// Command csplit is a thin wrapper around the csplit core utility.
package main

import (
	"charm.land/turtle/coreutils/csplit"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(csplit.New)
}
