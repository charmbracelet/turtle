# Hyphen-minus and colon (GNU Coreutils 9.12)
#### 30.3.1 Hyphen-minus ‘`-`’ and colon ‘`:`’

In Debian’s version string syntax the version consists of three parts:

```
[epoch:]upstream_version[-debian_revision]
```

The ‘`epoch`’ and ‘`debian_revision`’ parts are optional.

Example of such version strings:

```
60.7.2esr-1~deb9u1
52.9.0esr-1~deb9u1
1:2.3.4-1+b2
327-2
1:1.0.13-3
2:1.19.2-1+deb9u5
```

If the ‘`debian_revision part`’ is not present,
hyphens ‘`-`’ are not allowed.
If epoch is not present, colons ‘`:`’ are not allowed.

If these parts are present, hyphen and/or colons can appear only once
in valid Debian version strings.

In GNU Coreutils, such restrictions are not reasonable (a file name can
have many hyphens, a line of text can have many colons).

As a result, in GNU Coreutils hyphens and colons are treated exactly
like all other punctuation, i.e., they are sorted after
letters. See [Version sort punctuation](Version-sort-punctuation.md).

In Debian, these characters are treated differently than in Coreutils:
a version string with hyphen will sort before similar strings without
hyphens.

Compare:

```
$ touch 1ab-cd 1abb
$ ls -v -1
1abb
1ab-cd
$ if dpkg --compare-versions 1abb lt 1ab-cd
> then echo sorted
> else echo out of order
> fi
out of order
```

For further details, see [Comparing two strings using Debian’s algorithm](Comparing-two-strings-using-Debian_0027s-algorithm.md) and [GNU Bug 35939](https://bugs.gnu.org/35939).
