# String tests (GNU Coreutils 9.12)
#### 16.3.4 String tests

These options test string characteristics. You may need to quote
string arguments for the shell. For example:

```
test -n "$V"
```

The quotes here prevent the wrong arguments from being passed to
`test` if ‘`$V`’ is empty or contains special characters.

‘`-z string`’
:   True if the length of string is zero.

‘`-n string`’

‘`string`’
:   True if the length of string is nonzero.

‘`string1 = string2`’
:   True if the strings are equal.

‘`string1 == string2`’
:   True if the strings are equal (synonym for ‘`=`’).
    This form is not as portable to other
    shells and systems.

‘`string1 != string2`’
:   True if the strings are not equal.

‘`string1 > string2`’
:   True if string1 is greater than string2 in the current locale.

‘`string1 < string2`’
:   True if string1 is less than string2 in the current locale.
