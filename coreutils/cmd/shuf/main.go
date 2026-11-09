// Command shuf is a thin wrapper around the shuf core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/shuf"
)

func main() {
	cmdutil.Main(shuf.New)
}
