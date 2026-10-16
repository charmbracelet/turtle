# charm.land/snail/coreutils

A Go implementation of the Unix core utilities.

This is not meant to be perfect ports of each coreutil. Instead, it's meant to
have "good enough" compatibility to be embbeded in Go applications, usually when
running a shell interpreter.

I'm aiming for basic implementation based on the POSIX specifications first.
We can follow with some GNU extensions, specially the most common additions.

## Reasoning

There are other Go projects out there with implementations of the Unix core
utilities. Yet, this project started because we need to fit a specific use case.

[`mvdan.cc/sh`](https://github.com/mvdan/sh) is a great shell toolset (parser,
formatter and interpreter). It's used by projects like
[Task](https://github.com/go-task/task) and
[Crush](https://github.com/charmbracelet/crush) to have a way to execute shell
commands with a common dialect on all operating systems. In particular, it's
useful to bring compatibility to Windows where something like Bash not natively
supported.

The limitation is that it does not include the core utilities (`cp`, `mkdir` and
many others), only some built-in functions. On Windows, these are usually not
available in the system.
This project was built specifically to allow including these core utilities on
the shell interpreter, having it all compiled in a single Go binary (your app).

These are same of our needs that other implementations don't fit:

* The coreutils should be importable packages. Standalone `main` packages are
  not useful because we can't import them as libraries.
* Each coreutils package should have the same API design / interface to allow
  reference them without too much glue code.
* The implementation should use an isolated context from the actual application
  (Go app) running it. The working directory, file system, environment
  variables, stdin/stdout/stderr, etc. should be isolated and not shared with
  the actual global state of the Go app or other shell interpreter instances
  running on other goroutines.

## Progress

> [!IMPORTANT]
> This is a working in progress. We'll be breaking compatibility while this is a
v0 and we do progress. We have some progress tracking below.

### Phase 1: Foundational

| Program    | POSIX | GNU |
| ---------- | ----- | --- |
| `true`     | ✅    | ❌  |
| `false`    | ✅    | ❌  |
| `echo`     | ✅    | ❌  |
| `pwd`      | ✅    | ❌  |
| `basename` | ✅    | ❌  |
| `dirname`  | ✅    | ❌  |
| `wc`       | ✅    | ❌  |

### Phase 2: Filesystem

| Program | POSIX | GNU |
| ------- | ----- | --- |
| `touch` | ✅    | ❌  |
| `chmod` | ✅    | ❌  |
| `cat`   | ✅    | ❌  |
| `head`  | ✅    | ❌  |
| `tail`  | ✅    | ❌  |
| `mkdir` | ✅    | ❌  |
| `rmdir` | ✅    | ❌  |
| `cp`    | ✅    | ❌  |
| `mv`    | ✅    | ❌  |
| `rm`    | ✅    | ❌  |
| `ln`    | ✅    | ❌  |

### Phase 3: Text

| Program | POSIX | GNU |
| ------- | ----- | --- |
| `cut`   | ❌    | ❌  |
| `join`  | ❌    | ❌  |
| `sort`  | ❌    | ❌  |
| `paste` | ❌    | ❌  |
| `tee`   | ❌    | ❌  |
| `tr`    | ❌    | ❌  |
| `uniq`  | ❌    | ❌  |

### Phase 4: Discovery / Filtering

| Program | POSIX | GNU |
| ------- | ----- | --- |
| `find`  | ❌    | ❌  |
| `grep`  | ❌    | ❌  |
| `xargs` | ❌    | ❌  |
