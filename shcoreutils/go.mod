module charm.land/turtle/shcoreutils

go 1.26.8

require (
	charm.land/turtle/coreutils v0.1.0
	mvdan.cc/sh/v3 v3.14.1-0.20261125142311-f2395c263723
)

require (
	charm.land/turtle/fsx v0.1.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.47.0 // indirect
)

replace charm.land/turtle/coreutils => ../coreutils
