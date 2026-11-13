// Command tail is a thin wrapper around the tail core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/tail"
)

func main() {
	cmdutil.Main(tail.New)
}
