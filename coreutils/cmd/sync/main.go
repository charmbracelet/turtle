// Command sync is a thin wrapper around the sync core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/sync"
)

func main() {
	cmdutil.Main(sync.New)
}
