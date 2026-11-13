// Command gunzip is a thin wrapper around the gunzip core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/gunzip"
)

func main() {
	cmdutil.Main(gunzip.New)
}
