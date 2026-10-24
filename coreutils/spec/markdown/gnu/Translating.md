# Translating (GNU Coreutils 9.12)
#### 9.1.2 Translating

`tr` performs translation when string1 and string2 are
both given and the `--delete` (`-d`) option is not given.
`tr` translates each character of its input that is in array1
to the corresponding character in array2. Characters not in
array1 are passed through unchanged.

As a GNU extension to POSIX, when a character appears more than once
in array1, only the final instance is used. For example, these
two commands are equivalent:

```
tr aaa xyz
tr a z
```

A common use of `tr` is to convert lowercase characters to
uppercase. This can be done in many ways. Here are three of them:

```
tr abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ
tr a-z A-Z
tr '[:lower:]' '[:upper:]'
```

However, ranges like `a-z` are not portable outside the C locale.

When `tr` is performing translation, array1 and array2
typically have the same length. If array1 is shorter than
array2, the extra characters at the end of array2 are ignored.

On the other hand, making array1 longer than array2 is not
portable; POSIX says that the result is undefined. In this situation,
BSD `tr` pads array2 to the length of array1 by repeating
the last character of array2 as many times as necessary. System V
`tr` truncates array1 to the length of array2.

By default, GNU `tr` handles this case like BSD `tr`.
When the `--truncate-set1` (`-t`) option is given,
GNU `tr` handles this case like the System V `tr`
instead. This option is ignored for operations other than translation.

Acting like System V `tr` in this case breaks the relatively common
BSD idiom:

```
tr -cs A-Za-z0-9 '\012'
```

because it converts only zero bytes (the first element in the
complement of array1), rather than all non-alphanumerics, to
newlines.

By the way, the above idiom is not portable because it uses ranges, and
it assumes that the octal code for newline is 012. Here is a better
way to write it:

```
tr -cs '[:alnum:]' '[\n*]'
```
