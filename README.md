# charm.land/snail

Snail is a collection of shell tools and libraries in Go.

> [!IMPORTANT]
> This is a new repository so most stuff in here is a Work in Progress :tm:

## [charm.land/snail/coreutils](/coreutils)

A brand-new implementation of coreutils in Go. I'm aiming for POSIX compatibility
first, potentially following-up with GNU extensions later.

This was built to be importable as Go module to be used on shell interpreters
like [mvdan.cc/sh](https://github.com/mvdan/sh). This shell interpreter is used
on projects like [Task](https://github.com/go-task/task) and
[Crush](https://github.com/charmbracelet/crush).

## [charm.land/snail/fsx](/fsx)

A superset interface of [`io/fs.FS`](https://pkg.go.dev/io/fs#FS) that also
supports more functionality. The original interface from the Go standard library
is read-only.

This was built to make the coreutils module possible.
