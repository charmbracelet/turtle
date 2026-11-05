// Command uname is a thin wrapper around the uname core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/uname"
)

func main() {
	cmdutil.Main(uname.New)
}
