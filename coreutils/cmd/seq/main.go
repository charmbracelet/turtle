// Command seq is a thin wrapper around the seq core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/seq"
)

func main() {
	cmdutil.Main(seq.New)
}
