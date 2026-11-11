# basenc invocation (GNU Coreutils 9.12)
### 3.7 `basenc`: Transform data into printable data

`basenc` transforms data read from a file, or standard input,
into (or from) various common encoding forms. The encoded form uses
printable ASCII characters to represent binary data.

Synopses:

```
basenc encoding [option]... [file]
basenc encoding --decode [option]... [file]
```

The encoding argument is required. If file is omitted,
`basenc` reads from standard input.
The `-w/--wrap`,`-i/--ignore-garbage`,
`-d/--decode` options of this command are precisely the same as
for `base64`. See [`base64`: Transform data into printable data](base64-invocation.md).

Supported encodings are:

‘`--base64`’
:   Encode into (or decode from with `-d/--decode`) base64 form.
    The format conforms to
    [RFC 4648#4](https://datatracker.ietf.org/doc/html/rfc4648#section-4).
    Equivalent to the `base64` command.

‘`--base64url`’
:   Encode into (or decode from with `-d/--decode`) file-and-url-safe
    base64 form (using ‘`_`’ and ‘`-`’ instead of ‘`+`’ and ‘`/`’).
    The format conforms to
    [RFC 4648#5](https://datatracker.ietf.org/doc/html/rfc4648#section-5).

‘`--base58`’
:   Encode into (or decode from with `-d/--decode`) base58 form.
    The format conforms to
    [Base58 draft](https://datatracker.ietf.org/doc/html/draft-msporny-base58-03).
    This encoding is useful for transcription as the output avoids
    visually similar characters. It’s best suited to smaller amounts of data.
    For example this generates a unique 128 bit ID in 22 bytes:

    ```
    uuidgen | basenc --base16 -di | basenc --base58
    ```

‘`--base32`’
:   Encode into (or decode from with `-d/--decode`) base32 form.
    The encoded data uses the ‘`ABCDEFGHIJKLMNOPQRSTUVWXYZ234567=`’ characters.
    The format conforms to
    [RFC 4648#6](https://datatracker.ietf.org/doc/html/rfc4648#section-6).
    Equivalent to the `base32` command.

‘`--base32hex`’
:   Encode into (or decode from with `-d/--decode`) Extended Hex Alphabet
    base32 form. The encoded data uses the
    ‘`0123456789ABCDEFGHIJKLMNOPQRSTUV=`’ characters. The format conforms to
    [RFC 4648#7](https://datatracker.ietf.org/doc/html/rfc4648#section-7).

‘`--base16`’
:   Encode into (or decode from with `-d/--decode`) base16 (hexadecimal)
    form. The encoded data uses the ‘`0123456789ABCDEF`’ characters. The format
    conforms to
    [RFC 4648#8](https://datatracker.ietf.org/doc/html/rfc4648#section-8).

‘`--base2lsbf`’
:   Encode into (or decode from with `-d/--decode`) binary string form
    (‘`0`’ and ‘`1`’) with the *least* significant bit of every byte
    first.

‘`--base2msbf`’
:   Encode into (or decode from with `-d/--decode`) binary string form
    (‘`0`’ and ‘`1`’) with the *most* significant bit of every byte
    first.

‘`--z85`’
:   Encode into (or decode from with `-d/--decode`) Z85 form
    (a modified Ascii85 form). The encoded data uses the
    ‘`0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTU VWXYZ.-:+=^!/*?&<>()[]{}@%$#`’.
    characters. The format conforms to
    [ZeroMQ spec:32/Z85](https://rfc.zeromq.org/spec:32/Z85/).

    When encoding with `--z85`, input length must be a multiple of 4;
    when decoding with `--z85`, input length must be a multiple of 5.

Encoding/decoding examples:

```
$ printf '\376\117\202' | basenc --base64
/k+C

$ printf '\376\117\202' | basenc --base64url
_k-C

$ printf '\376\117\202' | basenc --base32
7ZHYE===

$ printf '\376\117\202' | basenc --base32hex
VP7O4===

$ printf '\376\117\202' | basenc --base16
FE4F82

$ printf '\376\117\202' | basenc --base2lsbf
011111111111001001000001

$ printf '\376\117\202' | basenc --base2msbf
111111100100111110000010

$ printf '\376\117\202\000' | basenc --z85
@.FaC

$ printf 01010100 | basenc --base2msbf --decode
T

$ printf 01010100 | basenc --base2lsbf --decode
*
```
