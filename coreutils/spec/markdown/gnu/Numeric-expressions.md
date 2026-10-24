# Numeric expressions (GNU Coreutils 9.12)
#### 16.4.2 Numeric expressions

`expr` supports the usual numeric operators, in order of increasing
precedence. These numeric operators have lower precedence than the
string operators described in the previous section, and higher precedence
than the connectives (next section).

‘`+ -`’
:   Addition and subtraction. Both arguments are converted to integers;
    an error occurs if this cannot be done.

‘`* / %`’
:   Multiplication, division, remainder. Both arguments are converted to
    integers; an error occurs if this cannot be done.
