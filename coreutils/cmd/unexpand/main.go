// Command unexpand is a thin wrapper around the unexpand core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/unexpand"
)

func main() {
	cmdutil.Main(unexpand.New)
}
