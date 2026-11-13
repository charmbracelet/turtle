// Command cat is a thin wrapper around the cat core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/cat"
)

func main() {
	cmdutil.Main(cat.New)
}
