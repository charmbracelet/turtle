# Related source code (GNU Coreutils 9.12)
#### 30.4.3 Related source code

* Debian’s code which splits a version string into
  `epoch/upstream_version/debian_revision` parts:
  [parsehelp.c:parseversion()](https://git.dpkg.org/cgit/dpkg/dpkg.git/tree/lib/dpkg/parsehelp.c#n191).
* Debian’s code which performs the `upstream_version` comparison:
  [version.c](https://git.dpkg.org/cgit/dpkg/dpkg.git/tree/lib/dpkg/version.c#n140).
* Gnulib code (used by GNU Coreutils) which performs the version comparison:
  [filevercmp.c](https://git.savannah.gnu.org/cgit/gnulib.git/tree/lib/filevercmp.c).
