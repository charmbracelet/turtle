# sha2 utilities (GNU Coreutils 9.12)
### 6.7 sha2 utilities: Print or check SHA-2 digests

This is a legacy interface to the more modern `cksum` utility.
See [`cksum`: Print and verify file checksums](cksum-invocation.md).

The commands `sha224sum`, `sha256sum`,
`sha384sum` and `sha512sum` compute checksums of
various lengths (respectively 224, 256, 384 and 512 bits),
collectively known as the SHA-2 hashes.

If a file is specified as ‘`-`’ or if no files are given
`sha???sum` computes the checksum for the standard input.
`sha???sum` can also determine whether a file and checksum are
consistent. Synopsis:

```
sha???sum [option]... [file]...
```

`sha???sum` uses the ‘`Untagged output format`’
for each specified file, as described at [cksum output modes](cksum-output-modes.md).

The program accepts [cksum common options](cksum-common-options.md). Also see [Common options](Common-options.md).
