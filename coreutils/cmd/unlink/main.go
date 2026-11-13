// Command unlink is a thin wrapper around the unlink core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/unlink"
)

func main() {
	cmdutil.Main(unlink.New)
}
