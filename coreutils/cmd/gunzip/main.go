// Command gunzip is a thin wrapper around the gunzip core utility.
package main

import (
	"charm.land/snail/coreutils/gunzip"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(gunzip.New)
}
