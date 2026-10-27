# Rules

![Rules decide before anyone is asked: read-only commands you allow run at once, everything else asks you, what you deny and what onceroot denies itself never runs](decide.svg)

## The order

1. **The built-in list.** Shells, interpreters, command wrappers, privilege
   tools and editors are refused, whatever the rules file says.
2. **`/etc/onceroot/rules`,** first match wins: `allow` runs the command at
   once, `deny` refuses it.
3. **No match:** a card goes to a person.

Every outcome is journalled, the commands that run without asking included.

## The rules file

```text
allow|deny <absolute program path> [<argument pattern>...] [...]
```

- The path is the program as root resolves it (`/usr/bin/pacman`), never the
  name the agent typed.
- An argument pattern is a literal, or ends in `*` for a prefix match.
- A trailing `...` lets further arguments through. Without it the number of
  arguments must match exactly.
- `#` starts a comment.

The shipped file:

```text
allow /usr/bin/pacman -Q* ...
allow /usr/bin/systemctl status ...

deny /usr/bin/pacman -R* ...
```

End an allow rule with `...` only for a tool whose every flag is read-only.
`journalctl ...` would also let `--vacuum-time` through, which deletes logs.

A rules file that does not parse, or that anyone but root can write, stops
every request until it is fixed: a broken file must not silently drop a deny.

## The built-in list

These are refused because the card cannot show what they would really do. The
check looks at both the resolved program and the name the agent used, and also
at the name without a version (`python3.12` is `python`).

| Kind | Names | Why |
|---|---|---|
| shells and interpreters | `sh` `bash` `zsh` `fish` `dash` `ksh` `mksh` `tcsh` `csh` `busybox` `toybox` `python` `pypy` `perl` `ruby` `node` `nodejs` `deno` `bun` `php` `lua` `luajit` `tclsh` `wish` `osascript` | they run code the card does not hold, and a script file can change after you approve |
| command wrappers | `env` `xargs` `nice` `nohup` `timeout` `setsid` `chroot` `unshare` `nsenter` `systemd-run` `script` `watch` `flock` `stdbuf` `ionice` `taskset` `chrt` `time` `strace` `ltrace` `gdb` | they run another command that rules and risk labels would never see |
| privilege tools | `sudo` `su` `pkexec` `doas` `run0` `sudoedit` `visudo` `runuser` `setpriv` `capsh` `sg` `newgrp` | they hand out root themselves |
| editors | `vi` `vim` `nvim` `nano` `emacs` `ed` `micro` `helix` `hx` `kak` | they need a terminal onceroot does not give |

An agent that needs a script's effect asks for its commands one at a time.

## Risk labels

The card says what kind of change a command makes, in onceroot's words, not the
agent's: `installs packages`, `installs a local package file`, `removes
packages`, `writes system files`, `removes files recursively`, `recursive on
/`, `writes disks or partitions`, `opens an editor as root`, or `none
detected`. A label is a hint for the person; it decides nothing.

## Limits

Set in `/etc/onceroot/onceroot.conf` ([install.md](install.md#the-settings-file)):

- a card waits `ttl` (10 minutes), then expires;
- `max_open` (3) open cards and `max_per_hour` (30) requests per user;
- after `pause_after` (3) rejections in a row, that user's requests pause for
  `pause` (15 minutes);
- a command that runs longer than `command_limit` (30 minutes) is stopped, with
  every process it started.
