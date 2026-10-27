# Approval from your phone

Optional, and off by default. When it is on, each card also goes to one private
Telegram chat, so a request can be approved away from the desk. The dialog at
the desk still opens; whichever answer comes first decides.

**Run against Telegram itself** on 2026-10-27: a card sent to a private chat,
Approve, then the PIN and a code from the phone's authenticator, and the
command ran once ([measured.md](measured.md#2026-10-27-approval-from-a-phone)).
The parts that talk to Telegram and check the factors also run in our other
projects:

- the Telegram client is [rollcall](https://github.com/TAIPANBOX/rollcall)'s,
  which binds a chat with a code sent from a phone and delivers its messages on
  real routers;
- the PIN, the authenticator code and the lockout are
  [openwrt-mcp](https://github.com/TAIPANBOX/openwrt-mcp)'s, which approve
  high-impact router changes from Telegram in
  [hermes-openwrt](https://github.com/TAIPANBOX/hermes-openwrt), measured on a
  Flint 2 router.

The rest of the flow (the buttons, deleting the PIN message first, answering
one private chat only, a phone that cannot be reached) is also covered by tests
against a stand-in Telegram server.

## What it takes

- **Reject:** one tap.
- **Approve:** a tap, then one message with your PIN and the six-digit code from
  an authenticator app: `482913 503917`. The bot deletes that message before it
  checks anything.
- The PIN is 6 to 8 digits; only a salted hash is kept (PBKDF2). The code
  changes every 30 seconds and each one works once.
- Five wrong tries lock approval from the phone for 15 minutes.
- After Approve, the PIN and code are awaited for three minutes.

A stolen Telegram session has neither factor, a lost phone lacks the PIN, and
the agent can approve nothing either way.

## Setting it up

1. Make a bot with @BotFather and keep its token. Find your numeric id (from
   @userinfobot, for example).
2. Put the token where only root can read it:

   ```sh
   sudo install -m 600 /dev/null /etc/onceroot/telegram.token
   sudoedit /etc/onceroot/telegram.token
   ```

3. Choose a PIN and enrol the authenticator. Type the PIN without echo:

   ```sh
   read -rs PIN; echo "$PIN" | sudo oncerootd setup-phone; unset PIN
   ```

   It prints an `otpauth://` link once. Add it to your authenticator app (most
   take the link, or make a QR code of it on your own machine), and keep it
   nowhere else: whoever has it can make your codes.

4. In `/etc/onceroot/onceroot.conf`:

   ```text
   phone = telegram
   telegram_chat = <your numeric id>
   ```

5. `sudo systemctl restart oncerootd`.

`oncerootd` refuses to start when `phone = telegram` is set and the token or
the factors file is missing, or readable by anyone but root.

## What Telegram sees

The card's text, and your taps. It never sees the PIN or the code: that message
is deleted first, and neither is written to the journal or a log.

The bot answers only the configured private chat, sends plain text (no
formatting, no link previews) and talks only to `api.telegram.org`.
