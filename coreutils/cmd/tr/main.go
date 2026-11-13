// Command tr is a thin wrapper around the tr core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/tr"
)

func main() {
	cmdutil.Main(tr.New)
}
