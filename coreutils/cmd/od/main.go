// Command od is a thin wrapper around the od core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/od"
)

func main() {
	cmdutil.Main(od.New)
}
