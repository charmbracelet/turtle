// Command nohup is a thin wrapper around the nohup core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/nohup"
)

func main() {
	cmdutil.Main(nohup.New)
}
