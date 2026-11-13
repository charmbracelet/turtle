// Command tee is a thin wrapper around the tee core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/tee"
)

func main() {
	cmdutil.Main(tee.New)
}
