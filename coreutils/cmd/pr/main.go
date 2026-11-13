package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/pr"
)

func main() {
	cmdutil.Main(pr.New)
}
