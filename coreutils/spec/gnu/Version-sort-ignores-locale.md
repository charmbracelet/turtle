# Version sort ignores locale (GNU Coreutils 9.12)
#### 30.2.6 Version sort ignores locale

In version sort, Unicode characters are compared byte-by-byte according
to their binary representation, ignoring their Unicode value or the
current locale.

Most commonly, Unicode characters are encoded as UTF-8 bytes; for
example, GREEK SMALL LETTER ALPHA (U+03B1, ‘`α`’) is encoded as the
UTF-8 sequence ‘`0xCE 0xB1`’). The encoding is compared
byte-by-byte, e.g., first ‘`0xCE`’ (decimal value 206) then
‘`0xB1`’ (decimal value 177).

```
$ touch aa az "a%" "aα"
$ ls -1 -v
aa
az
a%
aα
```

Ignoring the first letter (‘`a`’) which is identical in all
strings, the compared values are:

‘`a`’ and ‘`z`’ are letters, and sort before
all other non-digits.

Then, percent sign ‘`%`’ (ASCII value 37) is compared to the
first byte of the UTF-8 sequence of ‘`α`’, which is 0xCE or 206). The
value 37 is smaller, hence ‘`a%`’ is listed before ‘`aα`’.
