# shred invocation (GNU Coreutils 9.12)
### 11.6 `shred`: Remove files more securely

`shred` overwrites devices or files, to help prevent even
extensive forensics from recovering the data.

Ordinarily when you remove a file (see [`rm`: Remove files or directories](rm-invocation.md)), its data
and metadata are not actually destroyed. Only the file’s directory
entry is removed, and the file’s storage is reclaimed only when no
process has the file open and no other directory entry links to the
file. And even if file’s data and metadata’s storage space is freed
for further reuse, there are undelete utilities that will attempt to
reconstruct the file from the data in freed storage, and that can
bring the file back if the storage was not rewritten.

On a busy system with a nearly-full device, space can get reused in a few
seconds. But there is no way to know for sure. And although the
undelete utilities and already-existing processes require insider or
superuser access, you may be wary of the superuser,
of processes running on your behalf, or of attackers
that can physically access the storage device. So if you have sensitive
data, you may want to be sure that recovery is not possible
by plausible attacks like these.

The best way to remove something irretrievably is to destroy the media
it’s on with acid, melt it down, or the like. For cheap removable media
this is often the preferred method. However, some storage devices
are expensive or are harder to destroy, so the `shred` utility tries
to achieve a similar effect non-destructively, by overwriting the file
with non-sensitive data.

The `shred` command relies on a **crucial assumption**:
that the file system and hardware overwrite data in place.
Although this is common and is the traditional
way to do things, many modern file system designs do not satisfy this
assumption. Exceptions include:

* Log-structured or journaled file systems, such as ext3/ext4 (in
  `data=journal` mode), Btrfs, NTFS, ReiserFS, XFS, ZFS, file
  systems supplied with AIX and Solaris, etc., when they are configured to
  journal data.
* File systems that write redundant data and carry on even if some writes
  fail, such as RAID-based file systems.
* File systems that make snapshots, such as Network Appliance’s NFS server.
* File systems that cache in temporary locations, such as NFS version 3
  clients.
* Compressed file systems.

For ext3 and ext4 file systems, `shred` is less effective
when the file system is in `data=journal`
mode, which journals file data in addition to just metadata. In both
the `data=ordered` (default) and `data=writeback` modes,
`shred` works as usual. The ext3/ext4 journaling modes can be changed
by adding the `data=something` option to the mount options for a
particular file system in the `/etc/fstab` file, as documented in
the `mount` man page (‘`man mount`’). Alternatively, if
you know how large the journal is, you can shred the journal by
shredding enough file data so that the journal cycles around and fills
up with shredded data.

If you are not sure how your file system operates, then you should assume
that it does not overwrite data in place, which means `shred` cannot
reliably operate on regular files in your file system.

Generally speaking, it is more reliable to shred a device than a file,
since this bypasses file system design issues mentioned above.
However, devices are also problematic for shredding, for reasons
such as the following:

* Solid-state storage devices (SSDs) typically do wear leveling to
  prolong service life, and this means writes are distributed to other
  blocks by the hardware, so “overwritten” data blocks are still
  present in the underlying device.
* Most storage devices map out bad blocks invisibly to
  the application; if the bad blocks contain sensitive data,
  `shred` won’t be able to destroy it.
* With some obsolete storage technologies,
  it may be possible to take (say) a floppy disk back
  to a laboratory and use a lot of sensitive (and expensive) equipment
  to look for the faint “echoes” of the original data underneath the
  overwritten data. With these older technologies, if the file has been
  overwritten only once, it’s reputedly not even that hard. Luckily,
  this kind of data recovery has become difficult, and there is no
  public evidence that today’s higher-density storage devices can be
  analyzed in this way.

  The `shred` command can use many overwrite passes,
  with data patterns chosen to
  maximize the damage they do to the old data.
  By default the patterns are designed for best effect on hard drives using
  now-obsolete technology; for newer devices, a single pass should suffice.
  For more details, see the source code and Peter Gutmann’s paper
  [Secure Deletion of Data from Magnetic and Solid-State Memory](https://www.cs.auckland.ac.nz/~pgut001/pubs/secure_del.html),
  from the proceedings of the Sixth USENIX Security Symposium (San Jose,
  California, July 22–25, 1996).

`shred` makes no attempt to detect or report these problems, just as
it makes no attempt to do anything about backups. However, since it is
more reliable to shred devices than files, `shred` by default does
not deallocate or remove the output file. This default is more suitable
for devices, which typically cannot be deallocated and should not be
removed.

Finally, consider the risk of backups and mirrors.
File system backups and remote mirrors may contain copies of the
file that cannot be removed, and that will allow a shredded file
to be recovered later. So if you keep any data you may later want
to destroy using `shred`, be sure that it is not backed up or mirrored.

```
shred [option]... file[...]
```

If file is a FIFO, socket, or terminal, `shred` skips it
and emit a diagnostic message because the contents of these file types
do not reside on disk.

The program accepts the following options. Also see [Common options](Common-options.md).

‘`-f`’

‘`--force`’
:   Override file permissions if necessary to allow overwriting.

‘`-n number`’

‘`--iterations=number`’
:   By default, `shred` uses 3 passes of
    overwrite. You can reduce this to save time, or increase it if you think it’s
    appropriate. After 25 passes all of the internal overwrite patterns will have
    been used at least once.

‘`--random-source=file`’
:   Use file as a source of random data used to overwrite and to
    choose pass ordering. See [Sources of random data](Random-sources.md).

‘`-s bytes`’

‘`--size=bytes`’
:   Shred the first bytes bytes of the file. The default is to shred
    the whole file. bytes can be followed by a size specification like
    ‘`K`’, ‘`M`’, or ‘`G`’ to specify a multiple. See [Block size](Block-size.md).

‘`-u`’

‘`--remove[=how]`’
:   After shredding a file, deallocate it (if possible) and then remove it.
    If a file has multiple links, only the named links will be removed.
    Often the file name is less sensitive than the file data, in which case
    the optional how parameter, supported with the long form option,
    gives control of how to more efficiently remove each directory entry.
    The ‘`unlink`’ parameter will just use a standard unlink call,
    ‘`wipe`’ will also first obfuscate bytes in the name, and
    ‘`wipesync`’ will also sync each obfuscated byte in the name to
    the file system.
    Although ‘`wipesync`’ is the default method, it can be expensive,
    requiring a sync for every character in every file. This can become
    significant with many files, or is redundant if your file system provides
    synchronous metadata updates.

‘`-v`’

‘`--verbose`’
:   Display to standard error all status updates as sterilization proceeds.
    This details each data and metadata operation as it is performed.

‘`-x`’

‘`--exact`’
:   By default, `shred` rounds the size of a regular file up to the next
    multiple of the file system block size to fully erase the slack space in
    the last block of the file. This space may contain portions of the current
    system memory on some systems for example.
    Use `--exact` to suppress that behavior.
    Thus, by default if you shred a 10-byte regular file on a system with 512-byte
    blocks, the resulting file will be 512 bytes long. With this option,
    shred does not increase the apparent size of the file.

‘`-z`’

‘`--zero`’
:   Normally, the last pass that `shred` writes is made up of
    random data. If this would be conspicuous on your storage device (for
    example, because it looks like encrypted data), or you just think
    it’s tidier, the `--zero` option adds an additional overwrite pass with
    all zero bits. This is in addition to the number of passes specified
    by the `--iterations` option.

You might use the following command to erase the file system you
created on a USB flash drive. This command typically takes several
minutes, depending on the drive’s size and write speed. On modern
storage devices a single pass should be adequate, and will take one
third the time of the default three-pass approach.

```
shred -v -n 1 /dev/sdd1
```

Similarly, to erase all data on a selected partition of
your device, you could give a command like the following.

```
# 1 pass, write pseudo-random data; 3x faster than the default
shred -v -n1 /dev/sda5
```

To be on the safe side, use at least one pass that overwrites using
pseudo-random data. I.e., don’t be tempted to use ‘`-n0 --zero`’,
in case some device controller optimizes the process of writing blocks
of all zeros, and thereby does not clear all bytes in a block.
Some SSDs may do just that.

A file of ‘`-`’ denotes standard output.
The intended use of this is to shred a removed temporary file.
For example:

```
i=$(mktemp)
exec 3<>"$i"
rm -- "$i"
echo "Hello, world" >&3
shred - >&3
exec 3>&-
```

However, the command ‘`shred - >file`’ does not shred the contents
of file, since the shell truncates file before invoking
`shred`. Use the command ‘`shred file`’ or (if using a
Bourne-compatible shell) the command ‘`shred - 1<>file`’ instead.

An exit status of zero indicates success,
and a nonzero value indicates failure.
