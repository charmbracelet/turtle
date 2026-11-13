// Command chgrp is a thin wrapper around the chgrp core utility.
package main

import (
	"charm.land/turtle/coreutils/chgrp"
	"charm.land/turtle/coreutils/internal/cmdutil"
)

func main() {
	cmdutil.Main(chgrp.New)
}
