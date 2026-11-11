# b2sum invocation (GNU Coreutils 9.12)
### 6.5 `b2sum`: Print or check BLAKE2 digests

`b2sum` computes a 512-bit checksum for each specified
file.

This is a legacy interface to the more modern `cksum` utility.
See [`cksum`: Print and verify file checksums](cksum-invocation.md).

If a file is specified as ‘`-`’ or if no files are given
`b2sum` computes the checksum for the standard input.
`b2sum` can also determine whether a file and checksum are
consistent. Synopsis:

```
b2sum [option]... [file]...
```

`b2sum` uses the ‘`Untagged output format`’
for each specified file, as described at [cksum output modes](cksum-output-modes.md).

The program accepts [cksum common options](cksum-common-options.md). Also see [Common options](Common-options.md).

In addition `b2sum` supports the following options.

‘`-l bits`’

‘`--length=bits`’
:   Specify the digest size used by the algorithm. This option is optional.
    By default a 512 bit digest will be used. If the option is given it
    must be a multiple of 8.

    This option is ignored when `--check` is specified,
    as the length is automatically determined when checking.
