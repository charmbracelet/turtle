// Command mkdir is a thin wrapper around the mkdir core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/mkdir"
)

func main() {
	cmdutil.Main(mkdir.New)
}
