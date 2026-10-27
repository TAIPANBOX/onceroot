# Agents

An agent gets root through onceroot in one of two ways: it runs the CLI, or it
calls the MCP tool. Either way it asks for one exact command and waits for a
person.

## The CLI

```sh
onceroot run --why "install hyperfine to time the test suite" -- pacman -S --noconfirm hyperfine
```

- Everything after `--` is the command, one argument per word, exactly as it
  will run. There is no shell: no pipes, no `&&`, no globbing.
- A bare name is looked up in root's `PATH`
  (`/usr/local/sbin:/usr/local/bin:/usr/bin`), never the agent's. A name with a
  slash must be an absolute path.
- `--why` is one line the person reads under the card, quoted, as the agent's
  words.
- The card goes to the agent's standard error, then the command's own output
  follows once it runs.
- The exit code is the command's own. `126` means it did not run: refused,
  rejected, expired or withdrawn, with the reason on standard error.

Pressing Ctrl+C, or the agent's process ending, withdraws the card.

## The skill

The package installs a skill that tells an agent all of the above in a page:
`/usr/share/onceroot/skill/onceroot/SKILL.md`. Copy it where your agent reads
skills, for example:

```sh
mkdir -p ~/.claude/skills/onceroot
cp /usr/share/onceroot/skill/onceroot/SKILL.md ~/.claude/skills/onceroot/
```

## The MCP server

`onceroot mcp` is an MCP server over stdio with one tool, `run_as_root`. In
Claude Code:

```sh
claude mcp add onceroot -- onceroot mcp
```

Other agents take a stdio server whose command is `onceroot mcp`.

| | |
|---|---|
| input | `argv`: the program and its arguments, a list of strings; `why`: one line for the person |
| result | text with the exit code, the command's standard output and standard error (the card is in there) |
| `isError` | true when the command did not run, or exited with a code other than 0 |
| cancellation | `notifications/cancelled` for a waiting call withdraws its card; no answer is sent for it |

## What works well

- Non-interactive flags (`--noconfirm`, `-y`). Commands get no input.
- One command per request. Three rejections in a row pause the agent's requests
  for 15 minutes.
- Reading the refusal and changing the request, rather than asking for the same
  thing again.
- Short, plain arguments. A card the approval dialog cannot show whole is
  refused; paths relative to the folder on the card keep it short.

## What the agent cannot do

- Approve anything. The socket takes requests, and approval comes only from the
  person: polkit's dialog, `onceroot review` (which asks polkit) or the phone
  with a PIN and a code.
- Run a shell, an interpreter or a wrapper: [rules.md](rules.md#the-built-in-list).
- Go around onceroot through sudo, as long as passwordless sudo is off.
