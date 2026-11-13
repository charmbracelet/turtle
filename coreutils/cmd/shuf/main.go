// Command shuf is a thin wrapper around the shuf core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/shuf"
)

func main() {
	cmdutil.Main(shuf.New)
}
