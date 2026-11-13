// Command time is a thin wrapper around the time core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/time"
)

func main() {
	cmdutil.Main(time.New)
}
