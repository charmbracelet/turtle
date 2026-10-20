// Command tar is a thin wrapper around the tar core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/tar"
)

func main() {
	cmdutil.Main(tar.New)
}
