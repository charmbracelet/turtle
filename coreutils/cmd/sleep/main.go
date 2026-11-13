// Command sleep is a thin wrapper around the sleep core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/sleep"
)

func main() {
	cmdutil.Main(sleep.New)
}
