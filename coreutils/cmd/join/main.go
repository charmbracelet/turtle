// Command join is a thin wrapper around the join core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/join"
)

func main() {
	cmdutil.Main(join.New)
}
