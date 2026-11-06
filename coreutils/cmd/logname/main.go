// Command logname is a thin wrapper around the logname core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/logname"
)

func main() {
	cmdutil.Main(logname.New)
}
