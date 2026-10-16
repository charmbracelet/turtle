// Command head is a thin wrapper around the head core utility.
package main

import (
	"charm.land/snail/coreutils/head"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(head.New)
}
