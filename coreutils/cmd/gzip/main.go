// Command gzip is a thin wrapper around the gzip core utility.
package main

import (
	"charm.land/snail/coreutils/gzip"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(gzip.New)
}
