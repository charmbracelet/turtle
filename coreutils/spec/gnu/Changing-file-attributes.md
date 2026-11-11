# Changing file attributes (GNU Coreutils 9.12)
## 13 Changing file attributes

A file is not merely its contents, a name, and a file type
(see [Special file types](Special-file-types.md)). A file also has an owner (a user ID), a
group (a group ID), permissions (what the owner can do with the file,
what people in the group can do, and what everyone else can do), various
timestamps, and other information. Collectively, we call these a file’s
*attributes*.

These commands change file attributes.

* [`chown`: Change file owner and group](chown-invocation.md)
* [`chgrp`: Change group ownership](chgrp-invocation.md)
* [`chmod`: Change access permissions](chmod-invocation.md)
* [`touch`: Change file timestamps](touch-invocation.md)
