# charm.land/turtle/shcoreutils

An [mvdan/sh](https://github.com/mvdan/sh) interpreter middleware that runs
[our coreutils](../coreutils/README.md) inside Go programs.

This is particularly useful on Windows, where the Unix core utilities are not
normally installed. Utilities provided by the interpreter as builtins keep their
builtin behavior. Nested invocations through `xargs`, `env`, `find -exec` and
similar utilities also resolve coreutils and use the interpreter's remaining
execution handlers for external programs.

```go
package main

import (
	"log"

	"charm.land/turtle/shcoreutils"
	"mvdan.cc/sh/v3/interp"
)

func main() {
	interpreter, err := interp.New(interp.ExecHandlers(shcoreutils.New()))
	if err != nil {
		log.Fatal(err)
	}
	_ = interpreter
}
```

By default the Turtle implementations shadow the system ones. Use
`shcoreutils.New` with `shcoreutils.WithPriority` to change that:

- `PriorityTurtle` (default): always run the Turtle implementations.
- `PrioritySystem`: run the system utility when it resolves in PATH,
  falling back to Turtle otherwise.
- `PriorityAuto`: Turtle on Windows, system utilities elsewhere.

On Windows, a few programs whose system namesakes are unrelated tools
(`find`, `sort`, `expand`, `timeout`) always run as Turtle, whatever the
priority.

```go
connector := shcoreutils.New(shcoreutils.WithPriority(shcoreutils.PriorityAuto))
interpreter, err := interp.New(interp.ExecHandlers(connector))
```

Install it with:

```bash
go get charm.land/turtle/shcoreutils@latest
```
