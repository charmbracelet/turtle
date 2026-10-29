// Command link is a thin wrapper around the link core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/link"
)

func main() {
	cmdutil.Main(link.New)
}
