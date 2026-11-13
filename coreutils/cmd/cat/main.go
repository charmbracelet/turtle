// Command cat is a thin wrapper around the cat core utility.
package main

import (
	"charm.land/turtle/coreutils/cat"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(cat.New)
}
