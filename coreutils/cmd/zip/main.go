// Command zip is a thin wrapper around the zip core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/zip"
)

func main() {
	cmdutil.Main(zip.New)
}
