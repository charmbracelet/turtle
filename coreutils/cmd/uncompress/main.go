// Command uncompress is a thin wrapper around the POSIX uncompress core
// utility, restoring files instead of compressing them.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/uncompress"
)

func main() {
	cmdutil.Main(uncompress.New)
}
