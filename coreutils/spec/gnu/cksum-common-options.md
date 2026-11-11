# cksum common options (GNU Coreutils 9.12)
#### 6.3.3 cksum common options

‘`-b`’

‘`--binary`’
:   Treat each input file as binary, by reading it in binary mode and
    outputting a ‘`*`’ flag. This is the inverse of `--text`.
    On systems like GNU that do not distinguish between binary
    and text files, this option merely flags each input mode as binary:
    the checksum is unaffected. This option is the default on systems
    like MS-DOS that distinguish between binary and text files, except
    for reading standard input when standard input is a terminal.
    This option is deprecated and not documented in `cksum --help`.
    It is only supported by the `cksum` command to support emulation
    of the legacy standalone checksumming utilities.

‘`-c`’

‘`--check`’
:   Read file names and checksum information (not data) from each
    file (or from standard input if no file was specified) and report
    whether the checksums match the contents of the named files.
    The input to this mode is usually the output of a prior,
    checksum-generating run of the command. However the
    `cksum` command does not support `--check`
    with the legacy output format from the ‘`sysv`’, ‘`bsd`’,
    ‘`crc`’ or ‘`crc32b`’ algorithms.

    Three input formats are supported. Either the “untagged” output format,
    the “tagged” output format, or the BSD reversed mode format
    which is similar to the “untagged” output format
    but doesn’t use a character to distinguish binary and text modes.
    See [cksum output modes](cksum-output-modes.md) for details of these formats.

    For the `cksum` command, the `--check` option
    supports auto-detecting the digest algorithm to use,
    when presented with checksum information in the `--tag` output format.

    Also for the `cksum` command, the `--check` option
    auto-detects the digest encoding, accepting both standard hexadecimal
    checksums and those generated via `cksum` with its
    `--base64` option.

    Output with `--zero` enabled is not supported by `--check`.

    For each such line, `cksum` reads the named file and computes its
    checksum. Then, if the computed message digest does not match the
    one on the line with the file name, the file is noted as having
    failed the test. Otherwise, the file passes the test.
    By default, for each valid line, one line is written to standard
    output indicating whether the named file passed the test.
    After all checks have been performed, if there were any failures,
    a warning is issued to standard error.
    If file names contain problematic characters,
    they will be quoted in a way suitable for POSIX-compatible shells,
    so that any file name is represented unambiguously and safely.

    Use the `--status` option to inhibit the output described above.
    If any listed file cannot be opened or read, if any valid line has
    a checksum inconsistent with the associated file, or if no valid
    line is found, `cksum` exits with nonzero status. Otherwise,
    it exits successfully.

‘`--ignore-missing`’
:   This option is useful only when verifying checksums.
    When verifying checksums, don’t fail or report any status
    for missing files. This is useful when verifying a subset
    of downloaded files given a larger list of checksums.

‘`--quiet`’
:   This option is useful only when verifying checksums.
    When verifying checksums, don’t generate an ’OK’ message per successfully
    checked file. Files that fail the verification are reported in the
    default one-line-per-file format. If there is any checksum mismatch,
    print a warning summarizing the failures to standard error.

‘`--status`’
:   This option is useful only when verifying checksums.
    When verifying checksums, don’t generate the default one-line-per-file
    diagnostic and don’t output the warning summarizing any failures.
    Failures to open or read a file still evoke individual diagnostics to
    standard error.
    If all listed files are readable and are consistent with the associated
    checksums, exit successfully. Otherwise exit with a status code
    indicating there was a failure.

‘`--tag`’
:   Output BSD style checksums, which indicate the checksum algorithm used.
    As a GNU extension, if `--zero` is not used, file names with problematic
    characters are escaped as described above, using the same escaping indicator of
    ‘`\`’ at the start of the line, as used with the other output format.
    The `--tag` option implies binary mode, and is disallowed with
    `--text` mode as supporting that would unnecessarily complicate
    the output format, while providing little benefit.
    See [cksum output modes](cksum-output-modes.md) for details of this format.
    The `cksum` command, uses `--tag` as its default output format.

‘`-t`’

‘`--text`’
:   Treat each input file as text, by reading it in text mode and
    outputting a ‘’ flag. This is the inverse of `--binary`.
    This option is the default on systems like GNU that do not
    distinguish between binary and text files. On other systems, it is
    the default for reading standard input when standard input is a
    terminal. This mode is never defaulted to if `--tag` is used.
    This option is deprecated and not documented in `cksum --help`.
    It is only supported by the `cksum` command to support emulation
    of the legacy standalone checksumming utilities.

‘`-w`’

‘`--warn`’
:   When verifying checksums, warn about improperly formatted checksum lines.
    This option is useful only if all but a few lines in the checked input
    are valid.

‘`--strict`’
:   When verifying checksums,
    if one or more input line is invalid,
    exit nonzero after all warnings have been issued.

‘`-z`’

‘`--zero`’
:   Output a zero byte (ASCII NUL) at the end of each line,
    rather than a newline. This option enables other programs to parse the
    output even when that output would contain data with embedded newlines.
    Also file name escaping is not used.
