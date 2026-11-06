// Command id is a thin wrapper around the id core utility.
package main

import (
	"charm.land/snail/coreutils/id"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(id.New)
}
