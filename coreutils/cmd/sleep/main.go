// Command sleep is a thin wrapper around the sleep core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sleep"
)

func main() {
	cmdutil.Main(sleep.New)
}
