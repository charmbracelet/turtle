# Turtle

<p>
  <img width="450" alt="Charm Turtle artwork" src="https://github.com/user-attachments/assets/3a6fac67-4d92-4ca8-bd98-e54cd0b776d1" /><br>
  <a href="https://github.com/charmbracelet/turtle/releases"><img src="https://img.shields.io/github/release/charmbracelet/turtle" alt="Latest Release"></a>
  <a href="https://github.com/charmbracelet/turtle/actions"><img src="https://github.com/charmbracelet/turtle/actions/workflows/build.yml/badge.svg" alt="Build Status"></a>
</p>

A collection of shell-related tools and libraries, in pure Go.

## [charm.land/turtle/coreutils](/coreutils/README.md)

[![Go Reference](https://pkg.go.dev/badge/charm.land/turtle/coreutils.svg)](https://pkg.go.dev/charm.land/turtle/coreutils)

A Go implementation of the Unix core utilities.
Things like `cp`, `mv`, `rm`, `mkdir`, `chmod` and _several_ others.
[See the full list here](/coreutils/README.md#compatibility-table).

The intention is not to aim for 100% compatibility, but have something still
powerful and flexible enough to be imported by other Go programs, in particular
when using a [shell interpreter][mvdan].

* Mostly compatible with POSIX and GNU
* Importable Go packages
* Isolated state (including filesystem)
* Can run 100% in memory
* Zero dependencies
* Pluggable into Go [shell interpreters][mvdan]
* Used by [Crush][crush], [Task][task] and soon others!

Run `task bin` to compile the binaries locally on `./bin`, or import on your Go
app like this:

```bash
go install charm.land/turtle/coreutils@latest
```

```go
package main

import (
    "context"
    "log"

    "charm.land/turtle/coreutils"
    "charm.land/turtle/coreutils/programs/cp"
    "charm.land/turtle/coreutils/programs/mv"
)

func main() {
    const workingDir = "" // empty means current

    rt := coreutils.SystemRuntime(workingDir)
    ctx := context.Background()

    if err := cp.New(rt).Run(ctx, "input.txt", "copy.txt"); err != nil {
        log.Fatal(err)
    }
    if err := mv.New(rt).Run(ctx, "copy.txt", "archive.txt"); err != nil {
        log.Fatal(err)
    }
}
```

Want to read more? [Check out the coreutils module here](/coreutils/README.md).

## [charm.land/turtle/fsx](/fsx/README.md)

[![Go Reference](https://pkg.go.dev/badge/charm.land/turtle/fsx.svg)](https://pkg.go.dev/charm.land/turtle/fsx)

A filesystem abstraction for Go. Built to power the core utilities module,
but generic enough to be useful to many other projects.

The file system interface from the Go standard library is read-only. This
package embeds it and extends it with write-like operations, keeping the read
side fully compatible with the stdlib.

* A superset interface of the Go's standard library's
  [`io/fs.FS`](https://pkg.go.dev/io/fs#FS)
* Extends the stdlib's interface to support write operations like: write,
  rename, remove, chmod, mkdir and others
* Default implementations:
  * System (delegates to real FS)
  * Memory FS
  * [`os.Root`][osroot] wrapper

```bash
go install charm.land/turtle/fsx@latest
```

* `fsx.SystemFS(workingDir)`: use the system's FS
* `fsx.RootFS(root)` wraps an `os.Root` making it compatible with `fsx.FS`
* `fsx.MemoryFS`: an in-memory FS

[Check out the full docs on the fsx module here](/coreutils/README.md).

## charm.land/turtle/*

More comming soon... :eyes:

## Contributing

See the [contributing guide](https://github.com/charmbracelet/turtle?tab=contributing-ov-file#contributing).

## Whatcha think?

We’d love to hear your thoughts on this project. Need help? We gotchu. You can
find us on:

- [Twitter](https://twitter.com/charmcli)
- [Bluesky](https://bsky.app/profile/charm.land)
- [The Fediverse](https://mastodon.social/@charmcli)
- [Slack](https://charm.land/slack)
- [Discord](https://charm.land/discord)


## License

[Apache 2](https://github.com/charmbracelet/turtle/raw/main/LICENSE)

---

Part of [Charm](https://charm.land).

<a href="https://charm.land/"><img alt="The Charm logo" width="400" src="https://stuff.charm.sh/charm-banner-softy.jpg" /></a>

Charm热爱开源 • Charm loves open source

[charm]: https://charm.land
[mvdan]: https://github.com/mvdan/sh
[crush]: https://github.com/charmbracelet/crush
[task]: https://taskfile.dev
[osroot]: https://pkg.go.dev/os#Root
