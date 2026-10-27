# onceroot: root for an AI agent, one approved command at a time

Working name until 2026-10-27: askroot.

Status: design, 2026-10-27. Nothing is built yet. `onceroot` is a working name.

Provenance. Lines marked `@decided 2026-10-27` paraphrase decisions taken in the
session of that date. Facts marked `@source` were read in the named primary
source on 2026-10-27. Everything else is `@claude`: my design, to be checked.

## 1. Intent

AI coding agents on a developer's Linux desktop now run unattended. Omarchy, the
first audience, starts every agent it launches in its "don't stop to ask" mode
(`@source` omarchy `bin/omarchy-agent:56-197`, commit 958254e). When such an
agent needs root (install a package, restart a service, edit a file under
`/etc`), the desktop offers two answers today:

1. a password prompt the agent cannot answer, so it hangs until someone notices;
2. "passwordless sudo" for the whole user for 1 to 1440 minutes or for good, a
   sudoers rule `<user> ALL=(ALL) NOTAFTER=... NOPASSWD: ALL` (`@source` omarchy
   `bin/omarchy-sudo-passwordless:375-391`, `manual/48-security.md:21-29`).

The second one is root for every process of the user, not for the agent's one
task. Five minutes of it is root for good: in those minutes any process can
write its own rule under `/etc/sudoers.d`, which the expiry timer does not
remove. Omarchy's own manual says so in its words (`48-security.md:29`).

onceroot replaces both answers with a third one: **the agent asks for one exact
command, a person sees that command and approves it, and the command runs once.**

- `@decided 2026-10-27`: build our own tool, made to fit Omarchy as closely as
  possible first; porting to Ubuntu and other distributions comes after.
- `@decided 2026-10-27`: reuse code we already wrote and proved where it fits
  (the held-action approval and PIN/TOTP factors of our openwrt-mcp fork, the
  single-use approval token of Wardryx, the signed journal and Telegram channel
  of rollcall) instead of starting from nothing.
- `@decided 2026-10-27`: approval from a phone ships in the first version as an
  optional Telegram module, off by default. The core works locally: approval at
  the desk, rules, journal.

## 2. Goals, non-goals, anti-goals

Goals:
- G1. An agent gets root only for a command a person approved, or one a rule
  the person wrote allows without asking.
- G2. The person sees exactly what will run: the full argument list, the
  resolved program path, the working directory, which process asked, and a risk
  label written by onceroot, never text written by the agent.
- G3. An approval runs one command once. It cannot be replayed, widened or
  reused for another command.
- G4. Approval at the desk uses what Omarchy already has: the polkit dialog of
  its shell, with fingerprint, FIDO2 or password (`@source` omarchy
  `manual/37-hardware-authentication.md`, `shell/plugins/polkit`).
- G5. Approval away from the desk (opt-in): the same card in Telegram, approved
  with PIN and a TOTP code.
- G6. Every request, decision and execution is in a signed, hash-chained
  journal that a separate command verifies.
- G7. Works for every agent Omarchy launches, without depending on any one
  agent's hooks: the agent calls a CLI or an MCP tool.

Non-goals (others already do these, `@source` research 2026-10-27):
- A sandbox for the agent process, a network egress filter, or a credential
  proxy (kernel sandbox tools and the agents' own sandboxes do these).
- Rolling back config changes in the home directory (other tools do this).
- A sudo replacement for people. A person at a terminal keeps using sudo.
- Shell strings. onceroot runs an argument list, never `sh -c "<string>"` built
  by the agent.
- Interactive commands in v1: no TTY, no stdin.

Anti-goals (would make it a failure even if it works):
- A path where the agent, or anything running as the user, can approve its own
  request.
- A card whose text the agent wrote.
- Getting root more easily than with today's passwordless sudo, silently.
- An approval that, once given, holds for more than the one command.

## 3. Parts

| Part | What | Runs as |
|---|---|---|
| `oncerootd` | daemon: takes requests on a Unix socket, applies rules, holds cards, asks polkit, talks to Telegram when enabled, executes, journals | root, systemd service |
| `onceroot` | CLI: `run -- <argv>`, `status`, `list`, `approve <id>`, `reject <id>`, `journal verify`, `mcp` (stdio MCP server with one tool) | the user |
| polkit policy | action `org.taipanbox.onceroot.run`, `auth_admin` (never `_keep`) for active, inactive and any | installed file |
| rules file | `/etc/onceroot/rules`: allow without asking, always deny; root-owned | read by the daemon |
| Telegram module | opt-in, inside the daemon; bot token and TOTP secret in root-only files | root |
| Omarchy plugin | bar widget and panel: pending cards, recent journal lines; buttons call `onceroot approve/reject`, which go through polkit | the user, inside omarchy-shell |
| agent skill | tells agents to use `onceroot run --` instead of `sudo`; installed into the agents' skill directories | file |
| AUR package | binaries, unit, policy, rules example, sysusers/tmpfiles | |

## 4. Flow

1. The agent runs `onceroot run --why "install ripgrep" -- pacman -S --noconfirm ripgrep`
   (or calls the MCP tool `run_as_root` with the same fields).
2. The CLI connects to `/run/onceroot/onceroot.sock`. The daemon reads the
   peer's uid, gid and pid with `SO_PEERCRED` and the pid's start time from
   `/proc/<pid>/stat` (pid reuse defence). A uid not in the allowed list is
   refused.
3. The daemon resolves `argv[0]` against a fixed root `PATH`
   (`/usr/local/sbin:/usr/local/bin:/usr/bin`), records the absolute path and
   the SHA-256 of that file, and records the requester's working directory
   (`/proc/<pid>/cwd`). The environment of the command will be a fixed minimal
   set, never the requester's.
4. Rules, first match wins: `deny` refuses at once; `allow` runs without asking;
   no match means ask. Shell interpreters with `-c`, `env`, `xargs`, `sudo`,
   `su`, `pkexec`, editors and `visudo` carry a default deny that a rule cannot
   turn into allow (see 6.4).
5. To ask, the daemon creates a held card: id (5 letters), requester (uid,
   pid, start time, process name and parent chain as observed), cwd, resolved
   path, binary hash, argv, the agent's `why` shown as quoted and labelled
   "the agent says", risk labels, created and expiry times (10 minutes). The
   card hash is SHA-256 over all of these. The record is never edited; only its
   status moves: held -> approved -> ran | rejected | expired.
6. Desk channel: the daemon runs `pkcheck --action-id org.taipanbox.onceroot.run
   --process <pid>,<start>,<uid> --allow-user-interaction --detail
   polkit.message <card text>`. The user's polkit agent (Omarchy's shell) shows
   the card text and asks for fingerprint, FIDO2 or password. pkcheck's exit
   status decides.
7. Phone channel (when enabled): the card goes to the one allowed private chat
   as plain text with Approve and Reject buttons. Approve asks for PIN and code
   in the next message, which the bot deletes before anything else; the daemon
   checks PIN (PBKDF2) and TOTP (RFC 6238, step spent once) against the card
   hash.
8. First decision wins; the other channel's prompt is withdrawn or ignored.
9. Before executing, the daemon checks again: card still approved and not run,
   not expired, binary hash unchanged, path still the same file. Then it runs
   the argv with the fixed environment, in the recorded cwd, as root, with a
   time limit, and streams stdout, stderr and the exit status back to the
   waiting CLI. Status becomes `ran`.
10. Every step is a journal entry: Ed25519-signed, carrying the previous
    entry's hash. `onceroot journal verify` checks the chain and the signatures.

## 5. Omarchy fit

- The polkit dialog is Omarchy's own (`shell/plugins/polkit/PolkitAgent.qml`).
  It shows a custom message verbatim unless it matches pkexec's sentence
  (`PolkitModel.js:20-24`), so the card text reaches the person as written.
  Revised 2026-10-27 after the VM run: the dialog centres every line on a
  one-line band just above the password field, so a wide line runs off both
  edges of the screen and lower lines cover the field. onceroot gives it a
  dialog card of at most 12 lines of 96 columns, wrapped between quoted
  arguments, lifts it above the field with empty lines, and refuses a request
  whose card does not fit.
- `omarchy-sudo-passwordless`: when a grant is active (a file
  `/etc/sudoers.d/99-omarchy-*nopasswd*`), `onceroot status` and every card say
  so, because the agent can bypass onceroot through sudo while it is on.
- Agents are reached through a skill file placed in the directories Omarchy
  already links its own skill into (`manual/17-ai.md`, "The Omarchy Skill"),
  and through MCP for agents that prefer tools.
- The plugin is a third-party shell plugin installed from git with
  `omarchy plugin add` (`manual/32-shell-plugins.md`); it never holds root.
- The daemon arrives as an AUR package, the only way to install a root service
  on Omarchy without a core PR.

## 6. Security model

### 6.1 Who could attack

- The agent itself, misled by prompt injection or simply wrong.
- Any other process running as the same user (a compromised dependency, a
  browser exploit): onceroot must not give it more than the agent gets.
- Someone with the user's Telegram account but not the PIN or the TOTP device.
- Not in scope: root, the kernel, someone at the unlocked desk.

### 6.2 Threats and answers

| # | Threat | Answer |
|---|---|---|
| A1 | The agent approves its own request | Approval only through polkit `auth_admin` (a person authenticates in the session's agent) or the Telegram path with PIN and TOTP that the daemon checks. `onceroot approve` is itself a polkit check, not a shortcut. The socket accepts requests, never decisions. |
| A2 | The card says one thing, another runs | The daemon builds the card from the argv it will run. The agent's `why` is shown only as quoted, labelled text. Execution re-checks the card hash. |
| A3 | The binary changes between approval and execution | Path resolved by root, SHA-256 recorded on the card and checked again just before exec; a mismatch refuses and journals. |
| A4 | Replay or reuse of an approval | One card, one execution; the status moves once; a second run of the same card is refused. |
| A5 | Confusing card text (control characters, bidi marks, very long args) | Arguments are shown quoted, non-printable and bidi runes escaped; a card longer than the limit is refused with that reason instead of being cut. |
| A6 | Approval fatigue, request floods | At most N open cards, M per hour per uid, a pause after K rejections in a row; rules let safe reads through without asking. |
| A7 | Commands that are a shell in disguise | Built-in deny list a rule cannot override: `sh`, `bash`, `zsh`, `fish`, `dash` with `-c`, interpreters with `-c` or `-e`, `env`, `xargs`, `sudo`, `su`, `pkexec`, `doas`, `visudo`, editors. High-risk labels for `pacman -U`, `systemctl edit`, writes under `/etc/sudoers*`, `chmod`/`chown` on `/`, `dd`, `mkfs`. |
| A8 | Another user on the machine sends requests | `SO_PEERCRED` uid must be in the allowed list; the socket is mode 0660, group `onceroot`. |
| A9 | pid reuse makes polkit ask about the wrong process | The subject passed to pkcheck carries pid, start time and uid; the daemon reads the start time at connect and refuses if it changed. |
| A10 | Telegram account taken over | PIN and TOTP on every approval; only one private chat id; PIN tries limited with lockout; codes spent once. The card is visible to Telegram's servers, said plainly in the docs. |
| A11 | Journal edited | Ed25519 chain with prev-hash; `journal verify` names the first broken entry. Limit, said plainly: root can rewrite the whole chain with the key; no outside witness in v1. |
| A12 | Daemon parser bugs | Request and rules parsers are small, stdlib only, fuzzed. |
| A13 | Passwordless sudo is on and the agent goes around onceroot | Detected and shown on every card and in status; onceroot cannot stop it. |
| A14 | No polkit agent in the requester's session (agent started by a timer, or over ssh) | pkcheck exits 2 and nobody was asked: the card waits for `onceroot review` or the phone, the agent is told so, and it does not count as a rejection (revised 2026-10-27; it was rejected before). |

### 6.3 Defaults

TTL 10 minutes; open cards 3; 30 per hour per uid; pause 15 minutes after 3
rejections in a row; command time limit 30 minutes; PIN 6 to 8 digits, PBKDF2
200 000 iterations; 5 failed factor tries give a 15 minute lockout. All values
are `@claude` and live in the root-owned config.

### 6.4 Invariants

Each becomes a CLAUDE.md bullet with what holds it.

- I1. No request runs as root without a matching allow rule or an approval
  recorded for that card.
- I2. The socket never accepts an approval; approvals come only from polkit or
  the Telegram path with PIN and TOTP.
- I3. A card runs at most once.
- I4. A card past its expiry is never approved or run.
- I5. The executed path and binary hash equal the card's; otherwise nothing runs.
- I6. Card text contains no raw control or bidi characters and is never
  truncated silently. Revised 2026-10-27: only printable ASCII and Latin,
  Greek and Cyrillic letters appear unescaped, the dialog card has no line
  wider than the dialog, and a request whose card the dialog cannot show whole
  is refused.
- I7. A built-in denied command is denied whatever the rules file says.
- I8. The command's environment is the fixed set, never the requester's.
- I9. Every decision and execution is journalled before the reply goes out;
  a journal write failure refuses the request.
- I10. `journal verify` fails on any changed, removed or reordered entry.
- I11. Requests from a uid outside the allowed list are refused.
- I12. Secrets (bot token, PIN hash, TOTP secret) never appear in card text,
  journal entries, error messages or logs.
- I13. The polkit action is `auth_admin` for every subject class, never
  `auth_admin_keep` or `yes`.
- I14. Telegram messages carry no parse_mode and no link previews; only the
  configured private chat is answered.
- I15. go.mod requires only allowed modules.

## 7. Testing

Tier T3: authorization and privilege, where a wrong answer is silent.

- Gherkin scenarios in `features/`, bound to named tests both ways by a gate.
- Every new test red first against the unfixed code.
- Fuzz tests for the request decoder, the rules parser and the card renderer.
- Mutation testing on the decision path (rules, card state machine, factor
  checks, hash comparison), each mutant named with the test that caught it.
- `scripts/gates-have-teeth.sh` plants each gate's fault.
- Where it runs: unit tests on the Mac; Linux-only parts (`SO_PEERCRED`,
  `/proc`, exec as root) in a Linux container; polkit and the Omarchy dialog
  only on a real Omarchy, which means a VM (needs the owner's yes before one is
  started on the test machine).

## 8. Left to the builder

- Final name (the owner chooses before anything is published).
- Rules file syntax: line based, stdlib only.
- Exact card wording and layout inside the polkit message limit.
- How the plugin learns about new cards (poll `onceroot list --json` or a
  read-only status socket).
- Ubuntu port: polkit and systemd are the same; packaging and the plugin are
  not.
