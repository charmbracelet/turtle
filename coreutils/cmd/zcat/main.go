// Command zcat is a thin wrapper around the zcat core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/zcat"
)

func main() {
	cmdutil.Main(zcat.New)
}
