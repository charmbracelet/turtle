// Command csplit is a thin wrapper around the csplit core utility.
package main

import (
	"charm.land/snail/coreutils/csplit"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(csplit.New)
}
