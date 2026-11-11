# Charset selection in ptx (GNU Coreutils 9.12)
#### 7.5.2 Charset selection

As it is set up now, `ptx` assumes that the input file is coded
using 8-bit characters, and it may not work well in multibyte locales.
In a single-byte locale, the default regular expression
for a keyword allows foreign or diacriticized letters. Keyword sorting,
however, is still crude; it obeys the underlying character set ordering
quite blindly.

The output of `ptx` assumes the locale’s character encoding.
For example, with `ptx`’s `-T` option, if the locale
uses the Latin-1 encoding you may need a LaTeX directive like
‘`\usepackage[latin1]{inputenc}`’ to render non-ASCII characters
correctly.

‘`-f`’

‘`--ignore-case`’
:   Fold lower case letters to upper case for sorting.
