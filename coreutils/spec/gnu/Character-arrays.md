# Character arrays (GNU Coreutils 9.12)
#### 9.1.1 Specifying arrays of characters

The string1 and string2 operands are not regular
expressions, even though they may look similar. Instead, they
merely represent arrays of characters. As a GNU extension to POSIX,
an empty string operand represents an empty array of characters.

The interpretation of string1 and string2 depends on locale.
GNU `tr` fully supports only safe single-byte locales,
where each possible input byte represents a single character.
Unfortunately, this means GNU `tr` will not handle commands
like ‘`tr ö Ł`’ the way you might expect,
since (assuming a UTF-8 encoding) this is equivalent to
‘`tr '\303\266' '\305\201'`’ and GNU `tr` will
simply transliterate all ‘`\303`’ bytes to ‘`\305`’ bytes, etc.
POSIX does not clearly specify the behavior of `tr` in locales
where characters are represented by byte sequences instead of by
individual bytes, or where data might contain invalid bytes that are
encoding errors. To avoid problems in this area, you can run
`tr` in a safe single-byte locale by using a shell command
like ‘`LC_ALL=C tr`’ instead of plain `tr`.

Although most characters simply represent themselves in string1
and string2, the strings can contain shorthands listed below,
for convenience. Some shorthands can be used only in string1 or
string2, as noted below.

Backslash escapes
:   The following backslash escape sequences are recognized:

    ‘`\a`’
    :   Bell (BEL, Control-G).

    ‘`\b`’
    :   Backspace (BS, Control-H).

    ‘`\f`’
    :   Form feed (FF, Control-L).

    ‘`\n`’
    :   Newline (LF, Control-J).

    ‘`\r`’
    :   Carriage return (CR, Control-M).

    ‘`\t`’
    :   Tab (HT, Control-I).

    ‘`\v`’
    :   Vertical tab (VT, Control-K).

    ‘`\ooo`’
    :   The eight-bit byte with the value given by ooo, which is the longest
        sequence of one to three octal digits following the backslash.
        For portability, ooo should represent a value that fits in eight bits.
        As a GNU extension to POSIX, if the value would not fit, then only the
        first two digits of ooo are used, e.g., ‘`\400`’
        is equivalent to ‘`\0400`’ and represents a two-byte sequence.

    ‘`\\`’
    :   A backslash.

    It is an error if no character follows an unescaped backslash.
    As a GNU extension, a backslash followed by a character not listed
    above is interpreted as that character, removing any special
    significance; this can be used to escape the characters
    ‘`[`’ and ‘`-`’ when they would otherwise be special.

Ranges
:   The notation ‘`m-n`’ expands to the characters
    from m through n, in ascending order. m should
    not collate after n; if it does, an error results. As an example,
    ‘`0-9`’ is the same as ‘`0123456789`’.

    GNU `tr` does not support the System V syntax that uses square
    brackets to enclose ranges. Translations specified in that format
    sometimes work as expected, since the brackets are often transliterated
    to themselves. However, they should be avoided because they sometimes
    behave unexpectedly. For example, ‘`tr -d '[0-9]'`’ deletes brackets
    as well as digits.

    Many historically common and even accepted uses of ranges are not fully
    portable. For example, on EBCDIC hosts using the ‘`A-Z`’
    range will not do what most would expect because ‘`A`’ through ‘`Z`’
    are not contiguous as they are in ASCII.
    One way to work around this is to use character classes (see below).
    Otherwise, it is most portable (and most ugly) to enumerate the members
    of the ranges.

Repeated characters
:   The notation ‘`[c*n]`’ in string2 expands to n
    copies of character c. Thus, ‘`[y*6]`’ is the same as
    ‘`yyyyyy`’. The notation ‘`[c*]`’ in string2 expands
    to as many copies of c as are needed to make array2 as long as
    array1. If n begins with ‘`0`’, it is interpreted in
    octal, otherwise in decimal. A zero-valued n is treated as if
    it were absent.

Character classes
:   The notation ‘`[:class:]`’ expands to all characters in
    the (predefined) class class. When the `--delete` (`-d`)
    and `--squeeze-repeats` (`-s`) options are both given, any
    character class can be used in string2. Otherwise, only the
    character classes `lower` and `upper` are accepted in
    string2, and then only if the corresponding character class
    (`upper` and `lower`, respectively) is specified in the same
    relative position in string1. Doing this specifies case conversion.
    Except for case conversion, a class’s characters appear in no particular order.
    The class names are given below; an error results when an invalid class
    name is given.

    `alnum`
    :   Letters and digits.

    `alpha`
    :   Letters.

    `blank`
    :   Horizontal whitespace.

    `cntrl`
    :   Control characters.

    `digit`
    :   Digits.

    `graph`
    :   Printable characters, not including space.

    `lower`
    :   Lowercase letters.

    `print`
    :   Printable characters, including space.

    `punct`
    :   Punctuation characters.

    `space`
    :   Horizontal or vertical whitespace.

    `upper`
    :   Uppercase letters.

    `xdigit`
    :   Hexadecimal digits.

Equivalence classes
:   The syntax ‘`[=c=]`’ expands to all characters equivalent to
    c, in no particular order. These equivalence classes are
    allowed in string2 only when `--delete` (`-d`) and
    `--squeeze-repeats` `-s` are both given.

    Although equivalence classes are intended to support non-English alphabets,
    there seems to be no standard way to define them or determine their
    contents. Therefore, they are not fully implemented in GNU `tr`;
    each character’s equivalence class consists only of that character,
    which is of no particular use.

Since ‘`[:class:]`’ and ‘`[=c=]`’ syntax
are similar to shell globbing syntax, all such class arguments should
be quoted appropriately, when running `tr` from the shell.
