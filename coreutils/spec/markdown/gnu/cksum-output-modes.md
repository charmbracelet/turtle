# cksum output modes (GNU Coreutils 9.12)
#### 6.3.1 cksum output modes

Legacy output format
:   `cksum` by default prints the POSIX standard CRC checksum
    for each file along with the number of bytes in the file,
    and the file name unless no arguments were given.
    The 32-bit CRC used is based on the polynomial used
    for CRC error checking in the ISO/IEC 8802-3:1996 standard (Ethernet).
    Similar output formats are used for the other legacy checksums
    selectable with `--algorithm=crc32b`, and
    `--algorithm=sysv` or `--algorithm=bsd`
    detailed at [`sum`: Print checksum and block counts](sum-invocation.md).

Tagged output format
:   With the `--algorithm` option selecting non legacy checksums,
    the `cksum` command defaults to output of the form:

    ```
    digest_name (file name) = digest
    ```

    The standalone checksum utilities can select this output
    mode by using the `--tag` option.

Untagged output format
:   With the `--untagged` option and the `--algorithm` option
    selecting non legacy checksums, the following output format is used.
    This is the default output format of the standalone checksum utilities.
    For each file, we print the checksum, a space, a flag indicating
    binary or text input mode, and the file name.
    Binary mode is indicated with ‘`*`’, text mode with ‘’ (space).
    Binary mode is the default on systems where it’s significant,
    otherwise text mode is the default.

Without `--zero`, and with non legacy output formats,
if file contains a backslash, newline, or carriage return,
the line is started with a backslash, and each problematic character
in the file name is escaped with a backslash, making the output unambiguous
even in the presence of arbitrary file names.
Since the backslash character itself is escaped, any other backslash
escape sequences are reserved for future use.
