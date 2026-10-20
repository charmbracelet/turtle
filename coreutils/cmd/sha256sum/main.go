// Command sha256sum is a thin wrapper around the sha256sum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sha256sum"
)

func main() {
	cmdutil.Main(sha256sum.New)
}
