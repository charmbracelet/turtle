// Command truncate is a thin wrapper around the truncate core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/truncate"
)

func main() {
	cmdutil.Main(truncate.New)
}
