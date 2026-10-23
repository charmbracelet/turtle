package main

import (
	"charm.land/snail/coreutils/internal/cmdutil"
	"charm.land/snail/coreutils/pr"
)

func main() {
	cmdutil.Main(pr.New)
}
