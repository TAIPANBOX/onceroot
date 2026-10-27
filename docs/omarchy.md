# onceroot on Omarchy

Not released; this is how the pieces fit once the package is installed.

## Bar module

Add a custom command module to the bar in `~/.config/omarchy/shell.json`,
for example in `bar.layout.right`:

```json
{ "id": "onceroot", "type": "command", "exec": "onceroot bar", "interval": 3,
  "tooltip": "onceroot", "onClick": "omarchy-launch-floating-terminal-with-presentation onceroot review" }
```

`onceroot bar` prints nothing while no request waits and a padlock with the
number of waiting requests, highlighted, with the
cards in the tooltip when N requests wait. Clicking opens `onceroot review`,
which shows each card and takes `a` (approve: Omarchy's polkit dialog asks
for fingerprint, FIDO2 or password) or `r` (reject).

## Agents

Install `skill/onceroot/SKILL.md` where Omarchy links its own skill
(`~/.claude/skills`, `~/.codex/skills`, `~/.agents/skills`, ...). Agents that
use MCP can add `onceroot mcp` as a stdio server. Keep Omarchy's passwordless
sudo off: while it is on, any process of the user can go around onceroot, and
every card says so.
