// Command tail is a thin wrapper around the tail core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/tail"
)

func main() {
	cmdutil.Main(tail.New)
}
