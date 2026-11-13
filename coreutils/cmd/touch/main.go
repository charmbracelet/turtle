// Command touch is a thin wrapper around the touch core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/touch"
)

func main() {
	cmdutil.Main(touch.New)
}
