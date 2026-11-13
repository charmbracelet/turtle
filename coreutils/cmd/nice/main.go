// Command nice is a thin wrapper around the nice core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/nice"
)

func main() {
	cmdutil.Main(nice.New)
}
