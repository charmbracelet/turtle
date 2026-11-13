// Command uname is a thin wrapper around the uname core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/uname"
)

func main() {
	cmdutil.Main(uname.New)
}
