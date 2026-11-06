// Command nohup is a thin wrapper around the nohup core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/nohup"
)

func main() {
	cmdutil.Main(nohup.New)
}
