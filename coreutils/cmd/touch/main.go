// Command touch is a thin wrapper around the touch core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/touch"
)

func main() {
	cmdutil.Main(touch.New)
}
