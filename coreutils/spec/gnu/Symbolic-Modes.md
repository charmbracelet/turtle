# Symbolic Modes (GNU Coreutils 9.12)
### 27.2 Symbolic Modes

*Symbolic modes* represent changes to files’ mode bits as
operations on single-character symbols. They allow you to modify either
all or selected parts of files’ mode bits, optionally based on
their previous values, and perhaps on the current `umask` as well
(see [The Umask and Protection](Umask-and-Protection.md)).

The format of symbolic modes is:

```
[ugoa...][-+=]perms...[,...]
```

where perms is either zero or more letters from the set
‘`rwxXst`’, or a single letter from the set ‘`ugo`’.

The following sections describe the operators and other details of
symbolic modes.

* [Setting Permissions](Setting-Permissions.md)
* [Copying Existing Permissions](Copying-Permissions.md)
* [Changing Special Mode Bits](Changing-Special-Mode-Bits.md)
* [Conditional Executability](Conditional-Executability.md)
* [Making Multiple Changes](Multiple-Changes.md)
* [The Umask and Protection](Umask-and-Protection.md)
