// Command sha512sum is a thin wrapper around the sha512sum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sha512sum"
)

func main() {
	cmdutil.Main(sha512sum.New)
}
