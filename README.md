# charm.land/turtle

🐢 Turtle is a collection of shell-related tools and libraries in Go,
by [Charm][charm].

<img src="https://stuff.charm.land/turtle/turtle.png" />

## [charm.land/turtle/coreutils](/coreutils/README.md)

[![Go Reference](https://pkg.go.dev/badge/charm.land/turtle/coreutils.svg)](https://pkg.go.dev/charm.land/turtle/coreutils)

A Go implementation of the Unix core utilities.

* Mostly compatible with POSIX and GNU
* Importable Go packages
* Isolated state (including filesystem)
* Can run 100% in memory
* Zero dependencies
* Pluggable into Go [shell interpreters][mvdan]
* Used by [Crush][crush], [Task][task] and soon others!

## [charm.land/turtle/fsx](/fsx/README.md)

[![Go Reference](https://pkg.go.dev/badge/charm.land/turtle/fsx.svg)](https://pkg.go.dev/charm.land/turtle/fsx)

A filesystem abstraction for Go. Built to power the core utilities module,
but generic enough to be useful to many other projects.

* A superset interface of the standard library's
  [`io/fs.FS`](https://pkg.go.dev/io/fs#FS)
* Extends the stdlib's interface to support write operations like: write,
  rename, remove, chmod, mkdir and others
* Default implementations:
  * System (delegates to real FS)
  * Memory FS
  * [`os.Root`][osroot] wrapper

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
