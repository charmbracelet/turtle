# md5sum invocation (GNU Coreutils 9.12)
### 6.4 `md5sum`: Print or check MD5 digests

`md5sum` computes a 128-bit checksum (or *fingerprint* or
*message-digest*) for each specified file.

This is a legacy interface to the more modern `cksum` utility.
See [`cksum`: Print and verify file checksums](cksum-invocation.md).

The MD5 digest is more reliable than a simple CRC (provided by
the `cksum` command) for detecting accidental file corruption,
as the chances of accidentally having two files with identical MD5
are vanishingly small. However, it should not be considered secure
against malicious tampering: although finding a file with a given MD5
fingerprint is considered infeasible at the moment, it is known how
to modify certain files, including digital certificates, so that they
appear valid when signed with an MD5 digest. For more secure hashes,
consider using ‘`sha2`’, ‘`sha3`’, or ‘`blake2b`’,
available through the `cksum` `--algorithm` option.

If a file is specified as ‘`-`’ or if no files are given
`md5sum` computes the checksum for the standard input.
`md5sum` can also determine whether a file and checksum are
consistent. Synopsis:

```
md5sum [option]... [file]...
```

`md5sum` uses the ‘`Untagged output format`’
for each specified file, as described at [cksum output modes](cksum-output-modes.md).

The program accepts [cksum common options](cksum-common-options.md). Also see [Common options](Common-options.md).

An exit status of zero indicates success,
and a nonzero value indicates failure.
