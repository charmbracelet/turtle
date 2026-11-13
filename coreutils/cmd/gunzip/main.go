// Command gunzip is a thin wrapper around the gunzip core utility.
package main

import (
	"charm.land/turtle/coreutils/gunzip"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(gunzip.New)
}
