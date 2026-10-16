// Command cat is a thin wrapper around the cat core utility.
package main

import (
	"charm.land/snail/coreutils/cat"
	"charm.land/snail/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cat.New)
}
