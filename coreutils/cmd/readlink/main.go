// Command readlink is a thin wrapper around the readlink core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/readlink"
)

func main() {
	cmdutil.Main(readlink.New)
}
