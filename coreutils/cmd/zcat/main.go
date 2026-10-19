// Command zcat is a thin wrapper around the zcat core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/zcat"
)

func main() {
	cmdutil.Main(zcat.New)
}
