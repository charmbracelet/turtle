// Command truncate is a thin wrapper around the truncate core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/truncate"
)

func main() {
	cmdutil.Main(truncate.New)
}
