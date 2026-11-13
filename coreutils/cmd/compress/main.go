// Command compress is a thin wrapper around the POSIX compress core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/compress"
)

func main() {
	cmdutil.Main(compress.New)
}
