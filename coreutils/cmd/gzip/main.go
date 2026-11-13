// Command gzip is a thin wrapper around the gzip core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/gzip"
)

func main() {
	cmdutil.Main(gzip.New)
}
