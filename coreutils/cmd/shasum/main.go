// Command shasum is a thin wrapper around the shasum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/shasum"
)

func main() {
	cmdutil.Main(shasum.New)
}
