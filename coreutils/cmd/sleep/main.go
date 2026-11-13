// Command sleep is a thin wrapper around the sleep core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/sleep"
)

func main() {
	cmdutil.Main(sleep.New)
}
