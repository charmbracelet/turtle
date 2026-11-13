// Command true is a thin wrapper around the true core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/true"
)

func main() {
	cmdutil.Main(true.New)
}
