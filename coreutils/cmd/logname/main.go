// Command logname is a thin wrapper around the logname core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/logname"
)

func main() {
	cmdutil.Main(logname.New)
}
