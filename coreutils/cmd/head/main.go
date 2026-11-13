// Command head is a thin wrapper around the head core utility.
package main

import (
	"charm.land/turtle/coreutils/head"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(head.New)
}
