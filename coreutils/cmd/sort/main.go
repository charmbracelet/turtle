// Command sort is a thin wrapper around the sort core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sort"
)

func main() {
	cmdutil.Main(sort.New)
}
