# Multi-call invocation (GNU Coreutils 9.12)
### 2.15 `coreutils`: Multi-call program

The `coreutils` command invokes an individual utility, either
implicitly selected by the last component of the name used to invoke
`coreutils`, or explicitly with the
`--coreutils-prog` option. Synopsis:

```
coreutils --coreutils-prog=PROGRAM ...
```

The `coreutils` command is not installed by default, so
portable scripts should not rely on its existence.
