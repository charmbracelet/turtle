# sha1sum invocation (GNU Coreutils 9.12)
### 6.6 `sha1sum`: Print or check SHA-1 digests

`sha1sum` computes a 160-bit checksum for each specified file.

This is a legacy interface to the more modern `cksum` utility.
See [`cksum`: Print and verify file checksums](cksum-invocation.md).

The SHA-1 digest is more reliable than a simple CRC (provided by
the `cksum` command) for detecting accidental file corruption,
as the chances of accidentally having two files with identical SHA-1
are vanishingly small. However, it should not be considered secure
against malicious tampering: although finding a file with a given SHA-1
fingerprint is considered infeasible at the moment, it is known how
to modify certain files, including digital certificates, so that they
appear valid when signed with an SHA-1 digest. For more secure hashes,
consider using ‘`sha2`’, ‘`sha3`’, or ‘`blake2b`’,
available through the `cksum` `--algorithm` option.

If a file is specified as ‘`-`’ or if no files are given
`sha1sum` computes the checksum for the standard input.
`sha1sum` can also determine whether a file and checksum are
consistent. Synopsis:

```
sha1sum [option]... [file]...
```

`sha1sum` uses the ‘`Untagged output format`’
for each specified file, as described at [cksum output modes](cksum-output-modes.md).

The program accepts [cksum common options](cksum-common-options.md). Also see [Common options](Common-options.md).
