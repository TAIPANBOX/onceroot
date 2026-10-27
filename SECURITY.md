# Security

onceroot runs a root service that executes commands a person approved, and
keeps the factors for phone approval when that module is on. A flaw in that
path matters more than most bugs, so please report it privately.

## Reporting a vulnerability

Use GitHub's private report:
[Report a vulnerability](https://github.com/TAIPANBOX/onceroot/security/advisories/new).
Do not open a public issue or pull request for it.

Please include:

- what you did and what happened, with the commands, so it can be reproduced;
- the distribution and its version, and the desktop and polkit agent you use;
- the package version (`pacman -Q onceroot`) or the commit you built;
- `onceroot status` and the card the request showed, with private paths changed
  if you like.

Never send a real bot token, PIN or authenticator secret. A made-up one shows
the problem just as well.

A fix lands on `main`, and the advisory is published once the fix is there.

## In scope

- the service `oncerootd`, the CLI `onceroot`, the MCP server and the package
  this repository builds, with its unit, polkit action, sysusers and tmpfiles;
- anything that runs a command as root without a matching allow rule or a
  person's approval of exactly that command, runs it twice, or runs something
  other than what the card showed;
- a card or dialog that shows less than what runs, or that outside text can
  make misleading;
- the journal: an entry changed, removed or reordered without `onceroot
  journal verify` failing, by anyone but root;
- the phone module: an approval without the PIN and the code, a code used
  twice, a message from another chat that is acted on, or a secret reaching a
  log, the journal or Telegram.

## Out of scope

- root itself: root can stop the service, rewrite its files and its journal;
- what `README.md` lists under "What onceroot does not do".
