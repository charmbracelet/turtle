// Command sort is a thin wrapper around the sort core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/sort"
)

func main() {
	cmdutil.Main(sort.New)
}
