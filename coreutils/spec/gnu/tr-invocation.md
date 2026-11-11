# tr invocation (GNU Coreutils 9.12)
### 9.1 `tr`: Translate, squeeze, and/or delete characters

Synopsis:

```
tr [option]... string1 [string2]
```

`tr` copies standard input to standard output, performing
one of the following operations:

* translate, and optionally squeeze repeated characters in the result,
* squeeze repeated characters,
* delete characters,
* delete characters, then squeeze repeated characters from the result.

The string1 and string2 operands define arrays of
characters array1 and array2. By default array1
lists input characters that `tr` operates on, and array2
lists corresponding translations. In some cases the second operand is
omitted.

The program accepts the following options. Also see [Common options](Common-options.md).
Options must precede operands.

‘`-c`’

‘`-C`’

‘`--complement`’
:   Instead of array1, use its complement (all characters not
    specified by string1), in ascending order. Use this option with
    caution in multibyte locales where its meaning is not always clear
    or portable; see [Specifying arrays of characters](Character-arrays.md).

‘`-d`’

‘`--delete`’
:   Delete characters in array1; do not translate.

‘`-s`’

‘`--squeeze-repeats`’
:   Replace each sequence of a repeated character that is listed in
    the last specified array, with a single occurrence of that character.

‘`-t`’

‘`--truncate-set1`’
:   Truncate array1 to the length of array2.

An exit status of zero indicates success,
and a nonzero value indicates failure.

* [Specifying arrays of characters](Character-arrays.md)
* [Translating](Translating.md)
* [Squeezing repeats and deleting](Squeezing-and-deleting.md)
