// Command od is a thin wrapper around the od core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/od"
)

func main() {
	cmdutil.Main(od.New)
}
