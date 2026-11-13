// Command chgrp is a thin wrapper around the chgrp core utility.
package main

import (
	"charm.land/turtle/coreutils/internal/cmdutil"
	"charm.land/turtle/coreutils/programs/chgrp"
)

func main() {
	cmdutil.Main(chgrp.New)
}
