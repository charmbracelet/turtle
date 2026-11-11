# Access permission tests (GNU Coreutils 9.12)
#### 16.3.2 Access permission tests

These options test for particular access permissions.

‘`-g file`’
:   True if file exists and has its set-group-ID bit set.

‘`-k file`’
:   True if file exists and has its *sticky* bit set.

‘`-r file`’
:   True if file exists and the user has read access.

‘`-u file`’
:   True if file exists and has its set-user-ID bit set.

‘`-w file`’
:   True if file exists and the user has write access.

‘`-x file`’
:   True if file exists and the user has execute access
    (or search permission, if it is a directory).

‘`-O file`’
:   True if file exists and is owned by the current effective user ID.

‘`-G file`’
:   True if file exists and is owned by the current effective group ID.
