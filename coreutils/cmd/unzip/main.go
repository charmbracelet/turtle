// Command unzip is a thin wrapper around the unzip core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/unzip"
)

func main() {
	cmdutil.Main(unzip.New)
}
