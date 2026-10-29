// Command sync is a thin wrapper around the sync core utility.
package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/sync"
)

func main() {
	cmdutil.Main(sync.New)
}
