// Command uncompress is a thin wrapper around the POSIX uncompress core
// utility, restoring files instead of compressing them.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/uncompress"
)

func main() {
	cmdutil.Main(uncompress.New)
}
