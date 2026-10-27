---
name: onceroot
description: Use whenever a task needs root (installing packages, changing system services, editing files under /etc). Root comes only through onceroot, one approved command at a time.
---

# Getting root through onceroot

On this machine you do not use `sudo`, `pkexec`, `su` or `doas`. When a step
needs root, ask onceroot for that one command:

```
onceroot run --why "install ripgrep for the search task" -- pacman -S --noconfirm ripgrep
```

or call the MCP tool `run_as_root` with `argv` and `why`.

What happens:

- A person sees the exact command, the folder you are in, which process
  asked and your `why`, and approves or rejects it. The call waits. Ten
  minutes without an answer means expired.
- Some read-only commands run without asking; some commands are never run.
- Shells, interpreters and wrappers are always refused: `bash -c`, `sh`,
  `python -c`, `env`, `xargs`, `timeout`, `sudo` and the like. Ask for the
  real program directly, one command at a time.
- Give non-interactive flags (`--noconfirm`, `-y`): there is no terminal and
  no input.
- On refusal, read the reason in the output and change the request; do not
  retry the same command. Three rejections in a row pause your requests.

Keep `why` to one honest line. The person reads it next to the command;
it is shown as your words, quoted, and never replaces what the card says
will run.
