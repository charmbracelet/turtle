// Command time is a thin wrapper around the time core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/time"
)

func main() {
	cmdutil.Main(time.New)
}
