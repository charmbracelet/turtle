// Command md5sum is a thin wrapper around the md5sum core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/md5sum"
)

func main() {
	cmdutil.Main(md5sum.New)
}
