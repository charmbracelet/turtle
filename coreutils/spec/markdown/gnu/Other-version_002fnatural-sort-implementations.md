# Other version/natural sort implementations (GNU Coreutils 9.12)
#### 30.4.2 Other version/natural sort implementations

As previously mentioned, there are multiple variations on
version/natural sort, each with its own rules. Some examples are:

* Natural Sorting variants in
  [Rosetta Code](https://rosettacode.org/wiki/Natural_sorting).
* Python’s [natsort package](https://pypi.org/project/natsort/)
  (includes detailed description of their sorting rules:
  [natsort – how it works](https://natsort.readthedocs.io/en/master/howitworks.html)).
* Ruby’s [version_sorter](https://github.com/github/version_sorter).
* Perl has multiple packages for natural and version sorts
  (each likely with its own rules and nuances):
  [Sort::Naturally](https://metacpan.org/pod/Sort::Naturally),
  [Sort::Versions](https://metacpan.org/pod/Sort::Versions),
  [CPAN::Version](https://metacpan.org/pod/CPAN::Version).
* PHP has a built-in function
  [natsort](https://www.php.net/manual/en/function.natsort.php).
* NodeJS’s [natural-sort package](https://www.npmjs.com/package/natural-sort).
* In zsh, the
  [glob modifier](https://zsh.sourceforge.net/Doc/Release/Expansion.html#Glob-Qualifiers) ‘`*(n)`’ will expand to files in natural sort order.
* When writing C programs, the GNU libc library (‘`glibc`’)
  provides the
  [strverscmp(3)](https://man7.org/linux/man-pages/man3/strverscmp.3.html) function to compare two strings, and
  [versionsort(3)](https://man7.org/linux/man-pages/man3/versionsort.3.html)
  function to compare two directory entries (despite the names, they are
  not identical to GNU Coreutils version sort ordering).
* Using Debian’s sorting algorithm in:
  + python: [Stack Overflow Example #4957741](https://stackoverflow.com/a/4957741).
  + NodeJS: [deb-version-compare](https://www.npmjs.com/package/deb-version-compare).
