# Punctuation vs letters (GNU Coreutils 9.12)
#### 30.2.4 Punctuation vs letters

Rule 2.B.a says letters sort before non-letters
(after breaking down a string to digit and non-digit parts).

```
$ cat input6
a%
az
$ sort -V input6
az
a%
```

The input strings consist entirely of non-digits, and based on the
above algorithm have only one part, all non-digits
(‘`a%`’ vs ‘`az`’).

Each part is then compared lexically,
byte-by-byte; ‘`a`’ compares identically in both
strings.

Rule 2.B.a says a letter like ‘`z`’ sorts before
a non-letter like ‘`%`’ – hence ‘`az`’ appears first (despite
‘`z`’ having ASCII value of 122, much larger than ‘`%`’
with ASCII value 37).
