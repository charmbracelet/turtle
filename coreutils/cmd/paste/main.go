// Command paste is a thin wrapper around the paste core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/paste"
)

func main() {
	cmdutil.Main(paste.New)
}
