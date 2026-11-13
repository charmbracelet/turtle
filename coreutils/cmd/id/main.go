// Command id is a thin wrapper around the id core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/id"
)

func main() {
	cmdutil.Main(id.New)
}
