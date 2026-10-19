// Command compress is a thin wrapper around the POSIX compress core utility.
package main

import (
	"charm.land/snail/coreutils/compress"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(compress.New)
}
