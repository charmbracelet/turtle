// Command gzip is a thin wrapper around the gzip core utility.
package main

import (
	"charm.land/turtle/coreutils/gzip"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(gzip.New)
}
