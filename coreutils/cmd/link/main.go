// Command link is a thin wrapper around the link core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/link"
)

func main() {
	cmdutil.Main(link.New)
}
