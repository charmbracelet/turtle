# Padding and other flags (GNU Coreutils 9.12)
#### 21.1.1.4 Padding and other flags

Unless otherwise specified, `date` normally pads numeric fields
with zeros, so that, for
example, numeric months are always output as two digits.
Most numeric fields are padded on the left.
However, nanoseconds are padded on the right since they are commonly
used after decimal points in formats like ‘`%s.%-N`’.
Also, seconds since the Epoch are not padded
since there is no natural width for them.

The following optional flags can appear after the ‘`%`’:

‘`-`’
:   (hyphen) Do not pad the field; useful if the output is intended for
    human consumption.
    This is a GNU extension.
    As a special case, ‘`%-N`’ outputs only enough trailing digits to
    not lose information, assuming that the timestamp’s resolution is the
    same as the current hardware clock. For example, if the hardware
    clock resolution is 1 microsecond, ‘`%s.%-N`’ outputs something
    like ‘`1640890100.395710`’.

‘`_`’
:   (underscore) Pad with spaces; useful if you need a fixed
    number of characters in the output, but zeros are too distracting.
    This is a GNU extension.

‘`0`’
:   (zero) Pad with zeros even if the conversion specifier
    would normally pad with spaces.

‘`+`’
:   Pad with zeros, like ‘`0`’. In addition, precede any year number
    with ‘`+`’ if it exceeds 9999 or if its field width exceeds 4;
    similarly, precede any century number with ‘`+`’ if it exceeds 99
    or if its field width exceeds 2. This supports ISO 8601 formats
    for dates far in the future; for example, the command `date
    --date=12019-02-25 +%+13F` outputs the string ‘`+012019-02-25`’.

‘`^`’
:   Use upper case characters if possible.
    This is a GNU extension.

‘`#`’
:   Use opposite case characters if possible.
    A field that is normally upper case becomes lower case, and vice versa.
    This is a GNU extension.

Here are some examples of padding:

```
date +%d/%m -d "Feb 1"
⇒ 01/02
date +%-d/%-m -d "Feb 1"
⇒ 1/2
date +%_d/%_m -d "Feb 1"
⇒  1/ 2
```

You can optionally specify the field width
(after any flag, if present) as a decimal number. If the natural size of the
output of the field has less than the specified number of characters,
the result is normally written right adjusted and padded to the given
size. For example, ‘`%9B`’ prints the right adjusted month name in
a field of width 9. Nanoseconds are left adjusted, and are truncated
or padded to the field width.

An optional modifier can follow the optional flag and width
specification. The modifiers are:

‘`E`’
:   Use the locale’s alternate representation for date and time. This
    modifier applies to the ‘`%c`’, ‘`%C`’, ‘`%x`’, ‘`%X`’,
    ‘`%y`’ and ‘`%Y`’ conversion specifiers. In a Japanese locale, for
    example, ‘`%Ex`’ might yield a date format based on the Japanese
    Emperors’ reigns.

‘`O`’
:   Use the locale’s alternate numeric symbols for numbers. This modifier
    applies only to numeric conversion specifiers.

If the format supports the modifier but no alternate representation
is available, it is ignored.

POSIX specifies the behavior of flags and field widths only for
‘`%C`’, ‘`%F`’, ‘`%G`’, and ‘`%Y`’ (all without
modifiers), and requires a flag to be present if and only if a field
width is also present. Other combinations of flags, field widths and
modifiers are GNU extensions.
