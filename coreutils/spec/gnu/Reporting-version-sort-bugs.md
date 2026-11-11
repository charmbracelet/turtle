# Reporting version sort bugs (GNU Coreutils 9.12)
#### 30.4.1 Reporting version sort bugs

If you suspect a bug in GNU Coreutils version sort (i.e., in the
output of ‘`ls -v`’ or ‘`sort -V`’), please first check the following:

1. Is the result consistent with Debian’s own ordering (using `dpkg`, see
   [Comparing two strings using Debian’s algorithm](Comparing-two-strings-using-Debian_0027s-algorithm.md))? If it is, then this
   is not a bug – please do not report it.
2. If the result differs from Debian’s, is it explained by one of the
   sections in [Differences from Debian version sort](Differences-from-Debian-version-sort.md)? If it is,
   then this is not a bug – please do not report it.
3. If you have a question about specific ordering which is not explained
   here, please write to [coreutils@gnu.org](mailto:coreutils@gnu.org), and provide a
   concise example that will help us diagnose the issue.
4. If you still suspect a bug which is not explained by the above, please
   write to [bug-coreutils@gnu.org](mailto:bug-coreutils@gnu.org) with a concrete example of the
   suspected incorrect output, with details on why you think it is
   incorrect.
