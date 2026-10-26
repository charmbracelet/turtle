// Command join is a thin wrapper around the join core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/join"
)

func main() {
	cmdutil.Main(join.New)
}
