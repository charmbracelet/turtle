// Command seq is a thin wrapper around the seq core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/seq"
)

func main() {
	cmdutil.Main(seq.New)
}
