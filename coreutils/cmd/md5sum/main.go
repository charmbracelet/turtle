// Command md5sum is a thin wrapper around the md5sum core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/md5sum"
)

func main() {
	cmdutil.Main(md5sum.New)
}
