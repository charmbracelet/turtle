# cksum general options (GNU Coreutils 9.12)
#### 6.3.2 cksum general options

‘`-a type`’

‘`--algorithm=type`’
:   Compute checksums using the specified digest algorithm.

    Supported legacy checksums (which are not supported by `--check`):

    ```
    ‘sysv’      equivalent to sum -s
    ‘bsd’       equivalent to sum -r
    ‘crc’       equivalent to cksum (the default)
    ‘crc32b’    only available through cksum
    ```

    Supported more modern digest algorithms are:

    ```
    ‘md5’       equivalent to md5sum
    ‘sha1’      equivalent to sha1sum
    ‘sha2’      equivalent to sha{224,256,384,512}sum
    ‘sha3’      only available through cksum
    ‘blake2b’   equivalent to b2sum
    ‘sm3’       only available through cksum
    ```

    The following algorithms are currently considered secure against
    malicious tampering, i.e., there is no known way to modify a file to
    produce the same checksum:

    ```
    ‘sha2’      equivalent to sha{224,256,384,512}sum
    ‘sha3’      only available through cksum
    ‘blake2b’   equivalent to b2sum
    ```

‘`--base64`’
:   Print base64-encoded digests not hexadecimal.
    This option is ignored with `--check`.
    The format conforms to
    [RFC 4648#4](https://datatracker.ietf.org/doc/html/rfc4648#section-4).

    Each base64-encoded digest has zero, one or two trailing padding
    (‘`=`’) bytes. The length of that padding is the checksum-bit-length
    modulo 3, and the `--check` parser requires precisely the same
    input digest string as what is output. I.e., removing or adding any
    ‘`=`’ padding renders a digest non-matching.

‘`--debug`’
:   Output extra information to standard error,
    like the checksum implementation being used.

‘`-l bits`’

‘`--length=bits`’
:   Specify the digest size used with `-a sha2, sha3, or blake2b`.
    For ‘`blake2b`’ this is optional, with 512 being the default. If the
    option is given it must be a multiple of 8. For ‘`sha2`’ or ‘`sha3`’ this
    option is required, and the length must be one of 224, 256, 384, or 512.

    This option is ignored when `--check` is specified,
    as the length is automatically determined when checking.

‘`--raw`’
:   Print only the unencoded raw binary digest for a single input.
    Do not output the file name or anything else.
    Use network byte order (big endian) where applicable:
    for ‘`bsd`’, ‘`crc`’, ‘`crc32b`’, and ‘`sysv`’.
    This option works only with a single input.
    Unlike other output formats, `cksum` provides no way to
    `--check` a `--raw` checksum.

‘`--untagged`’
:   Output using the original Coreutils format used by the other
    standalone checksum utilities like `md5sum` for example.
    This format has the checksum at the start of the line, and may be
    more amenable to further processing by other utilities,
    especially in combination with the `--zero` option.
    This does not identify the digest algorithm used for the checksum.
    See [cksum output modes](cksum-output-modes.md) for details of this format.
