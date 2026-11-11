# Changing Special Mode Bits (GNU Coreutils 9.12)
#### 27.2.3 Changing Special Mode Bits

In addition to changing a file’s read, write, and execute/search permissions,
you can change its special mode bits. See [Structure of File Mode Bits](Mode-Structure.md), for a
summary of these special mode bits.

To change the file mode bits to set the user ID on execution, use
‘`u`’ in the users part of the symbolic mode and
‘`s`’ in the permissions part.

To change the file mode bits to set the group ID on execution, use
‘`g`’ in the users part of the symbolic mode and
‘`s`’ in the permissions part.

To set both user and group ID on execution, omit the users part
of the symbolic mode (or use ‘`a`’) and use ‘`s`’ in the
permissions part.

To change the file mode bits to set the restricted deletion flag or sticky bit,
omit the users part of the symbolic mode (or use ‘`a`’) and use
‘`t`’ in the permissions part.

For example, to set the set-user-ID mode bit of a program,
you can use the mode:

```
u+s
```

To remove both set-user-ID and set-group-ID mode bits from
it, you can use the mode:

```
a-s
```

To set the restricted deletion flag or sticky bit, you can use
the mode:

```
+t
```

The combination ‘`o+s`’ has no effect. On GNU systems
the combinations ‘`u+t`’ and ‘`g+t`’ have no effect, and
‘`o+t`’ acts like plain ‘`+t`’.

The ‘`=`’ operator is not very useful with special mode bits.
For example, the mode:

```
o=t
```

does set the restricted deletion flag or sticky bit, but it also
removes all read, write, and execute/search permissions that users not in the
file’s group might have had for it.

See [Directories and the Set-User-ID and Set-Group-ID Bits](Directory-Setuid-and-Setgid.md), for additional rules concerning
set-user-ID and set-group-ID bits and directories.
