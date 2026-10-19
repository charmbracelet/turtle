// Command sha224sum is a thin wrapper around the sha224sum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sha224sum"
)

func main() {
	cmdutil.Main(sha224sum.New)
}
