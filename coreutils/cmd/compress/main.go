// Command compress is a thin wrapper around the POSIX compress core utility.
package main

import (
	"charm.land/turtle/coreutils/compress"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(compress.New)
}
