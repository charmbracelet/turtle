// Command tar is a thin wrapper around the tar core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/tar"
)

func main() {
	cmdutil.Main(tar.New)
}
