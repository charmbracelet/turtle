// Command tr is a thin wrapper around the tr core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/tr"
)

func main() {
	cmdutil.Main(tr.New)
}
