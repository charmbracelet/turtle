// Command uniq is a thin wrapper around the uniq core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/uniq"
)

func main() {
	cmdutil.Main(uniq.New)
}
