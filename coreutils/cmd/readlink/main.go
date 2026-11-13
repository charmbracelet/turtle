// Command readlink is a thin wrapper around the readlink core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/readlink"
)

func main() {
	cmdutil.Main(readlink.New)
}
