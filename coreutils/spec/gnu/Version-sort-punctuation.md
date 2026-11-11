# Version sort punctuation (GNU Coreutils 9.12)
#### 30.2.3 Version sort punctuation

Punctuation is sorted by ASCII order (rule 2.B).

```
$ touch 1.0.5_src.tar.gz 1.0_src.tar.gz
$ ls -v -1
1.0.5_src.tar.gz
1.0_src.tar.gz
```

Why is `1.0.5_src.tar.gz` listed before `1.0_src.tar.gz`?

Based on the version-sort ordering rules, the strings are broken down
into the following parts:

```
          1   vs  1               (rule 3, all digits)
          .   vs  .               (rule 2, all non-digits)
          0   vs  0               (rule 3)
          .   vs  _src.tar.gz     (rule 2)
          5   vs  empty string    (no more bytes in the file name)
_src.tar.gz   vs  empty string
```

The fourth parts (‘`.`’ and ‘`_src.tar.gz`’) are compared
lexically by ASCII order. The ‘`.`’ (ASCII value 46) is
less than ‘`_`’ (ASCII value 95) – and should be listed before it.

Hence, `1.0.5_src.tar.gz` is listed first.

If a different byte appears instead of the underscore (for
example, percent sign ‘`%`’ ASCII value 37, which is less
than dot’s ASCII value of 46), that file will be listed first:

```
$ touch   1.0.5_src.tar.gz     1.0%zzzzz.gz
1.0%zzzzz.gz
1.0.5_src.tar.gz
```

The same reasoning applies to the following example, as ‘`.`’ with
ASCII value 46 is less than ‘`/`’ with ASCII value 47:

```
$ cat input5
3.0/
3.0.5
$ sort -V input5
3.0.5
3.0/
```
