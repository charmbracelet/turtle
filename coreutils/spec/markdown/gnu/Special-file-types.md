# Special file types (GNU Coreutils 9.12)
## 12 Special file types

This chapter describes commands which create special types of files (and
`rmdir`, which removes directories, one special file type).

Although Unix-like operating systems have markedly fewer special file
types than others, not *everything* can be treated only as the
undifferentiated byte stream of *normal files*. For example, when a
file is created or removed, the system must record this information,
which it does in a *directory* – a special type of file. Although
you can read directories as normal files, if you’re curious, in order
for the system to do its job it must impose a structure, a certain
order, on the bytes of the file. Thus it is a “special” type of file.

Besides directories, other special file types include named pipes
(FIFOs), symbolic links, sockets, and so-called *special files*.

* [`link`: Make a hard link via the link syscall](link-invocation.md)
* [`ln`: Make links between files](ln-invocation.md)
* [`mkdir`: Make directories](mkdir-invocation.md)
* [`mkfifo`: Make FIFOs (named pipes)](mkfifo-invocation.md)
* [`mknod`: Make block or character special files](mknod-invocation.md)
* [`readlink`: Print value of a symlink or canonical file name](readlink-invocation.md)
* [`rmdir`: Remove empty directories](rmdir-invocation.md)
* [`unlink`: Remove files via the unlink syscall](unlink-invocation.md)
