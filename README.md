# onceroot

Root for an AI agent, one approved command at a time.

Coding agents now work unattended on developer desktops. When one of them
needs root, a desktop usually offers two answers: a password prompt the agent
cannot answer, so it stalls, or passwordless sudo, which gives root to every
process of your user for minutes or for good. Five minutes of passwordless
sudo is root for good, because in those minutes any process can write its own
rule into `/etc/sudoers.d`.

onceroot is a third answer. The agent asks for one exact command. You see that
command and approve it. It runs once.

Not released yet. Built for [Omarchy](https://omarchy.org) first; any Linux
with systemd and polkit is the next step.

## What you see

When an agent asks, onceroot shows a card in the agent's terminal, and the
same facts in fewer lines in your desktop's authentication dialog:

```
ROOT REQUEST KKSQU (until 14:07)
Run: "/usr/bin/touch"
  "touch"
  "/etc/onceroot-e2e-marker"
In: "/home/tester"
Asked by: "onceroot" (pid 3405) <- "bash" <- "foot" <- "Hyprland"
Risk: writes system files
The agent says: "e2e-test"
```

Everything on the card comes from onceroot, not from the agent: the program as
root resolves it, every argument quoted (any character that could hide or fake
an argument shown escaped), the folder, the process chain read
from the kernel, and a risk label. The agent's own words appear only on the
last line, quoted. You approve with your fingerprint, a security key or your
password; Escape rejects.

## How it works

1. The agent runs `onceroot run --why "one line" -- program args...` instead of
   `sudo`, or calls the MCP tool `run_as_root`.
2. `oncerootd`, a small root service, reads who is asking from the kernel
   (`SO_PEERCRED`, `/proc`) and refuses users you did not list.
3. It resolves the program in root's own `PATH`, requires the file and every
   folder above it to belong to root, and records the file's SHA-256.
4. Rules decide what happens before anyone is asked: read-only commands you
   list run at once, commands you forbid never run, and shells, interpreters,
   command wrappers, privilege tools and editors are always refused, because
   a card cannot show the code they would run.
5. Anything else becomes a card that waits up to ten minutes for you.
6. Once approved, onceroot checks again that the asking process is still the
   same one and the program has not changed, then runs the argument list
   exactly, with a fixed environment, no input and a time limit.
7. Every request, decision and run goes into a signed, hash-chained journal
   that `onceroot journal verify` checks.

An approval holds for that one command, once. Nothing the agent sends over the
socket can approve anything: approvals come only from you.

## Install

There is no package yet; an AUR package `onceroot` is planned. From a checkout,
on Arch or Omarchy:

```
go build -o onceroot ./cmd/onceroot && go build -o oncerootd ./cmd/oncerootd
sudo install -Dm755 onceroot oncerootd -t /usr/bin
sudo install -Dm644 packaging/oncerootd.service /usr/lib/systemd/system/oncerootd.service
sudo install -Dm644 packaging/org.taipanbox.onceroot.policy /usr/share/polkit-1/actions/org.taipanbox.onceroot.policy
sudo install -Dm644 packaging/onceroot.sysusers /usr/lib/sysusers.d/onceroot.conf
sudo install -Dm644 packaging/onceroot.tmpfiles /usr/lib/tmpfiles.d/onceroot.conf
sudo install -Dm644 packaging/onceroot.conf /etc/onceroot/onceroot.conf
sudo install -Dm644 packaging/rules.example /etc/onceroot/rules
sudo systemd-sysusers && sudo systemd-tmpfiles --create
```

Then put your uid (`id -u`) into `allowed_uids` in `/etc/onceroot/onceroot.conf`,
join the group with `sudo usermod -aG onceroot $USER` (log in again), and start
the service with `sudo systemctl enable --now oncerootd`.

Keep passwordless sudo off. While it is on, any process of your user can go
around onceroot, and every card says so.

## Omarchy

On Omarchy the authentication dialog opens as soon as a request arrives
(`desk_prompt = auto`). A request whose card that dialog cannot show whole is
refused, and the agent is asked for shorter arguments: no dialog can approve
a command it does not show. Elsewhere onceroot does not rely on the dialog showing
the card; you approve from `onceroot review`, which prints the whole card
first. To see waiting requests in the bar, add a command module; see
[docs/omarchy.md](docs/omarchy.md).

## Agents

Give agents [skill/onceroot/SKILL.md](skill/onceroot/SKILL.md), and add
`onceroot mcp` as a stdio MCP server for agents that prefer tools. A cancelled
tool call withdraws its request.

## Rules

`/etc/onceroot/rules`, first match wins:

```
allow /usr/bin/pacman -Q* ...
allow /usr/bin/systemctl status ...
deny  /usr/bin/pacman -R* ...
```

A pattern is a literal or ends in `*`. A trailing `...` lets further arguments
through, so use it only for tools whose every flag is read-only. A rules file
that does not parse, or that others can write, stops every request.

## Approval from your phone (optional, off by default)

With `phone = telegram`, the card also goes to one private Telegram chat.
Reject takes one tap; Approve asks for your PIN and a code from an
authenticator app, and the message carrying them is deleted first. Telegram's
servers see the card. Set it up with `oncerootd setup-phone`.

## What onceroot does not do

- It does not sandbox the agent or filter its network. Use a sandbox for that;
  onceroot is about the moment the agent needs root.
- Root can rewrite the journal with its key; the journal shows tampering by
  anyone else.
- A command runs in the folder shown on the card, and that folder's contents
  can change after you approve.
- Any process of your own user can fill your request limits and make the agent
  wait.
- Commands get no input: give them non-interactive flags (`--noconfirm`, `-y`).
  A command that reads its input, such as `tee`, reads nothing.
- Commands see their own empty `/tmp` and cannot write home directories, so
  "build in /tmp, then install as root" does not work through onceroot; build
  in your project folder instead.
- The built-in refusal of shells, interpreters and wrappers goes by the
  program's file name. A copy of an interpreter that root installs under
  another name is not recognised; only root can put one there.

## Tested

What has been run, where and with what result is in
[docs/measured.md](docs/measured.md): the test suite on macOS and on Linux, the
service on Omarchy 4.0.6 in a virtual machine, a request approved and one
rejected in Omarchy's own dialog, and mutation testing of the decision path.

## License

MIT. Parts are ported from two other MIT projects; see [NOTICE](NOTICE).
