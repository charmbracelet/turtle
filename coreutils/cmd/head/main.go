// Command head is a thin wrapper around the head core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/head"
)

func main() {
	cmdutil.Main(head.New)
}
