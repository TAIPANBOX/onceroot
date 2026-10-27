# How it works

![An agent asks oncerootd for one command; oncerootd checks it and shows you a card; you approve; the command runs once and every step is journalled](hero.svg)

## One request, step by step

1. **The agent asks** for one command over the socket `/run/onceroot/onceroot.sock`
   (`0660 root:onceroot`), with `onceroot run` or the MCP tool.
2. **Who is asking** comes from the kernel, not from the request: the uid, pid
   and process start time through `SO_PEERCRED` and `/proc`, and the chain of
   parent processes. A uid outside `allowed_uids` is refused.
3. **What would run** is resolved the way root would: in root's own `PATH`, the
   symlinks followed to the real file. That file and every folder above it must
   belong to root and be writable by nobody else, and its SHA-256 goes on the
   card.
4. **The rules decide** before anyone is asked: the built-in list, then the
   rules file, first match wins ([rules.md](rules.md)).
5. **The card** is made from onceroot's own findings: program, every argument
   quoted, folder, process chain, risk label. The agent's words are one quoted
   line. The card is hashed; an approval is bound to that hash.
6. **A person decides**: the polkit dialog (`auth_admin` every time, never
   remembered), `onceroot review`, or the phone with a PIN and a code. Nothing
   sent over the socket can approve.
7. **Just before the run**, onceroot checks that the asking process is still
   the same one (pid and start time) and that the program has the same SHA-256.
   Otherwise nothing runs.
8. **The run** uses the argument list exactly, with a fixed environment
   (`PATH`, `LANG`, `HOME=/root`), no input, its own process group and a time
   limit. Output streams back to the agent.
9. **The journal** gets every step, written before the answer goes out: an
   Ed25519-signed entry carrying the hash of the one before. A journal that
   cannot be written refuses the request.

A card runs at most once, and never after it expires.

## The guarantees, and what holds each one

Each line is a rule in the code with the tests or gates that hold it; the full
list, with test names, is in [CLAUDE.md](../CLAUDE.md#invariants).

- No command runs as root without a matching allow rule or a person's approval
  of that card.
- The socket never approves; approval comes from polkit or from the phone with
  both factors.
- A card runs at most once, and not after it expires.
- The program that runs is the file and the content on the card.
- A card holds no raw control or direction-changing characters, is never cut
  short, and the dialog's version fits the dialog or the request is refused.
- A built-in refusal beats any rule.
- The command's environment is the fixed set, never the agent's.
- Every decision and run is journalled first, and `journal verify` fails on any
  changed, removed or reordered entry.
- Secrets (the bot token, the PIN's hash, the authenticator secret) never reach
  a card, the journal, an error or a log.
- The polkit action asks every time, for every kind of session.

## Limits

These are design limits, said where you meet them, not bugs:

- Root can rewrite the journal with its key; tampering by anyone else shows. A
  cut tail is not detected without a copy kept elsewhere.
- The folder on the card can change between your approval and the run.
- Any process of your own user can fill your limits and make the agent wait.
- A copy of an interpreter that root installs under another name is not on the
  built-in list.
- Commands see their own empty `/tmp` and cannot write home directories.

The design document, with the threat table these come from:
[docs/superpowers/specs/2026-10-27-onceroot-design.md](superpowers/specs/2026-10-27-onceroot-design.md).
