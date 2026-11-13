// Command mkdir is a thin wrapper around the mkdir core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/mkdir"
)

func main() {
	cmdutil.Main(mkdir.New)
}
