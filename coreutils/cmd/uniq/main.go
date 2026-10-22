// Command uniq is a thin wrapper around the uniq core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/uniq"
)

func main() {
	cmdutil.Main(uniq.New)
}
