// Command split is a thin wrapper around the split core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/split"
)

func main() {
	cmdutil.Main(split.New)
}
