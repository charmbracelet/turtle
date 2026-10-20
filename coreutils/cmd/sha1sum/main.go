// Command sha1sum is a thin wrapper around the sha1sum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sha1sum"
)

func main() {
	cmdutil.Main(sha1sum.New)
}
