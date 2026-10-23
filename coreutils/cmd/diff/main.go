// Command diff is a thin wrapper around the diff core utility.
package main

import (
	"charm.land/snail/coreutils/diff"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(diff.New)
}
