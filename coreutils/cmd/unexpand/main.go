// Command unexpand is a thin wrapper around the unexpand core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/unexpand"
)

func main() {
	cmdutil.Main(unexpand.New)
}
