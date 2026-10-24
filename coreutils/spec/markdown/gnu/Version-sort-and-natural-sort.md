# Version sort and natural sort (GNU Coreutils 9.12)
#### 30.1.2 Version sort and natural sort

In GNU Coreutils, the name *version sort* was chosen because it is based
on Debian GNU/Linux’s algorithm of sorting packages’ versions.

Its goal is to answer questions like
“Which package is newer, `firefox-60.7.2` or `firefox-60.12.3`?”

In Coreutils this algorithm was slightly modified to work on more
general input such as textual strings and file names
(see [Differences from Debian version sort](Differences-from-Debian-version-sort.md)).

In other contexts, such as other programs and other programming
languages, a similar sorting functionality is called
[natural sort](https://en.wikipedia.org/wiki/Natural_sort_order).
