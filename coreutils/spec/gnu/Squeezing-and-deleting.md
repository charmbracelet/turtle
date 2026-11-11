# Squeezing and deleting (GNU Coreutils 9.12)
#### 9.1.3 Squeezing repeats and deleting

When given just the `--delete` (`-d`) option, `tr`
removes any input characters that are in array1.

When given just the `--squeeze-repeats` (`-s`) option
and not translating, `tr` replaces each input sequence of a
repeated character that is in array1 with a single occurrence of
that character.

When given both `--delete` and `--squeeze-repeats`, `tr`
first performs any deletions using array1, then squeezes repeats
from any remaining characters using array2.

The `--squeeze-repeats` option may also be used when translating,
in which case `tr` first performs translation, then squeezes
repeats from any remaining characters using array2.

Here are some examples to illustrate various combinations of options:

* Remove all zero bytes:

  ```
  tr -d '\0'
  ```
* Put all words on lines by themselves. This converts all
  non-alphanumeric characters to newlines, then squeezes each string
  of repeated newlines into a single newline:

  ```
  tr -cs '[:alnum:]' '[\n*]'
  ```
* Convert each sequence of repeated newlines to a single newline.
  I.e., delete empty lines:

  ```
  tr -s '\n'
  ```
* Find doubled occurrences of words in a document.
  For example, people often write “the the” with the repeated words
  separated by a newline. The Bourne shell script below works first
  by converting each sequence of punctuation and blank characters to a
  single newline. That puts each “word” on a line by itself.
  Next it maps all uppercase characters to lower case, and finally it
  runs `uniq` with the `-d` option to print out only the words
  that were repeated.

  ```
  #!/bin/sh
  cat -- "$@" \
    | tr -s '[:punct:][:blank:]' '[\n*]' \
    | tr '[:upper:]' '[:lower:]' \
    | uniq -d
  ```
* Deleting a small set of characters is usually straightforward. For example,
  to remove all ‘`a`’s, ‘`x`’s, and ‘`M`’s you would do this:

  ```
  tr -d axM
  ```

  However, when ‘`-`’ is one of those characters, it can be tricky because
  ‘`-`’ has special meanings. Performing the same task as above but also
  removing all ‘`-`’ characters, we might try `tr -d -axM`, but
  that would fail because `tr` would try to interpret `-a` as
  a command-line option. Alternatively, we could try putting the hyphen
  inside the string, `tr -d a-xM`, but that wouldn’t work either because
  it would make `tr` interpret `a-x` as the range of characters
  ‘`a`’…‘`x`’ rather than the three.
  One way to solve the problem is to put the hyphen at the end of the list
  of characters:

  ```
  tr -d axM-
  ```

  Or you can use ‘`--`’ to terminate option processing:

  ```
  tr -d -- -axM
  ```
