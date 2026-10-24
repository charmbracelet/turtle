# Using version sort in GNU Coreutils (GNU Coreutils 9.12)
#### 30.1.1 Using version sort in GNU Coreutils

Two GNU Coreutils programs use version sort: `ls` and `sort`.

To list files in version sort order, use `ls`
with the `-v` or `--sort=version` option:

```
default sort:              version sort:

$ ls -1                    $ ls -1 -v
a1                         a1
a100                       a1.4
a1.13                      a1.13
a1.4                       a1.40
a1.40                      a2
a2                         a100
```

To sort text files in version sort order, use `sort` with
the `-V` or `--version-sort` option:

```
$ cat input
b3
b11
b1
b20

lexicographic order:       version sort order:

$ sort input               $ sort -V input
b1                         b1
b11                        b3
b20                        b11
b3                         b20
```

To sort a specific field in a file, use `-k/--key` with
‘`V`’ type sorting, which is often combined with ‘`b`’ to
ignore leading blanks in the field:

```
$ cat input2
100   b3   apples
2000  b11  oranges
3000  b1   potatoes
4000  b20  bananas
$ sort -k 2bV,2 input2
3000  b1   potatoes
100   b3   apples
2000  b11  oranges
4000  b20  bananas
```
