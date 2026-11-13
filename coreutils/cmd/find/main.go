// Command find is a thin wrapper around the find core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/find"
)

func main() {
	cmdutil.Main(find.New)
}
