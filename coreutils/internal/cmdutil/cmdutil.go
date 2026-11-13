// Package cmdutil helps the main packages of the core utilities.
package cmdutil

import (
	"context"
	"errors"
	"os"

	"charm.land/turtle/coreutils"
)

// Main runs the utility built by new against the process arguments, exiting
// the process with its status code.
func Main(new coreutils.NewFunc) {
	if code := run(new, os.Args[1:]); code != 0 {
		os.Exit(code)
	}
}

// run executes the utility built by new and returns its status code.
func run(new coreutils.NewFunc, args []string) int {
	if err := new(coreutils.OsRuntime()).Run(context.Background(), args...); err != nil {
		if exitErr, ok := errors.AsType[*coreutils.ExitError](err); ok {
			return exitErr.Code
		}
		return 1
	}
	return 0
}
