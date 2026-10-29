// Command unlink is a thin wrapper around the unlink core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/unlink"
)

func main() {
	cmdutil.Main(unlink.New)
}
