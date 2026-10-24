# cksum invocation (GNU Coreutils 9.12)
### 6.3 `cksum`: Print and verify file checksums

`cksum` by default computes a 32-bit cyclic redundancy check (CRC)
checksum for each given file, or standard input if none are given or for
a file of ‘`-`’.

cksum also supports the `-a/--algorithm` option to select the
digest algorithm to use. `cksum` is the preferred interface
to these digests, subsuming the other standalone checksumming utilities,
which can be emulated using `cksum -a md5 --untagged "$@"` etc.
Synopsis:

```
cksum [option]... [file]...
```

`cksum` is typically used to ensure that files have not been corrupted,
by comparing the `cksum` output for the received files with the
`cksum` output for the original files (typically given in the
distribution).

* [cksum output modes](cksum-output-modes.md)
* [cksum general options](cksum-general-options.md)
* [cksum common options](cksum-common-options.md)
