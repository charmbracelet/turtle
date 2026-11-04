// Command find is a thin wrapper around the find core utility.
package main

import (
	"charm.land/snail/coreutils/find"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(find.New)
}
