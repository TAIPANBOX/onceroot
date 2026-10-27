#!/usr/bin/env python3
"""Mutation testing of onceroot's decision path.

Each mutant breaks the logic the way a silent wrong answer would, in a
throwaway copy of the repository, and requires an existing test to fail.
A mutant that does not compile is INVALID, never counted as caught.
Run: python3 -I scripts/mutate.py
"""
import os
import re
import shutil
import subprocess
import sys
import tempfile

# name, file, old, new, package to test
MUTANTS = [
    ("card: approval ignores the hash", "internal/card/card.go", "\tif h != r.Card.Hash {", "\tif false {", "./internal/card/"),
    ("card: approval ignores expiry", "internal/card/card.go", "\tif !now.Before(r.Card.Expires) {\n\t\treturn ErrExpired\n\t}\n\tif h", "\tif false {\n\t\treturn ErrExpired\n\t}\n\tif h", "./internal/card/"),
    ("card: run ignores expiry", "internal/card/card.go", "\tcase !now.Before(r.Card.Expires):", "\tcase false:", "./internal/card/"),
    ("card: run leaves the card approved", "internal/card/card.go", "\tr.move(Ran, r.By, now)", "\tr.move(Approved, r.By, now)", "./internal/card/"),
    ("card: hash skips argv", "internal/card/card.go", "\twriteStrings(h, c.Argv)", "\t_ = c.Argv", "./internal/card/"),
    ("card: argv shown unquoted", "internal/card/render.go", 'b.WriteString("\\n  " + quote(a))', 'b.WriteString("\\n  " + a)', "./internal/card/"),
    ("rules: built-in deny skipped", "internal/rules/rules.go", "if deny, why := Builtin(path, argv); deny {", "if deny, why := Builtin(path, argv); false && deny {", "./internal/rules/"),
    ("rules: argument count ignored", "internal/rules/rules.go", "if len(args) < len(r.Args) || (!r.Rest && len(args) != len(r.Args)) {", "if len(args) < len(r.Args) {", "./internal/rules/"),
    ("rules: path ignored", "internal/rules/rules.go", "\tif r.Path != path {", "\tif false {", "./internal/rules/"),
    ("store: open limit off by one", "internal/store/store.go", "if open >= s.l.MaxOpen {", "if open > s.l.MaxOpen {", "./internal/store/"),
    ("store: pause off by one", "internal/store/store.go", "if u.rejectRun >= s.l.PauseAfter {", "if u.rejectRun > s.l.PauseAfter {", "./internal/store/"),
    ("factor: PIN not checked", "internal/factor/lockout.go", "\tif !CheckPIN(pin, f.PINHash, f.PINSalt) {", "\tif false {", "./internal/factor/"),
    ("factor: spent step accepted again", "internal/factor/totp.go", "\t\tif ctr <= t.last {", "\t\tif ctr < t.last {", "./internal/factor/"),
    ("factor: lockout off by one", "internal/factor/lockout.go", "\tif l.fails >= l.max {", "\tif l.fails > l.max {", "./internal/factor/"),
    ("journal: signature not checked", "internal/journal/journal.go", "\t\tif !sig.Verify(pub, signKind, signed, s) {", "\t\tif false {", "./internal/journal/"),
    ("journal: sequence not checked", "internal/journal/journal.go", "\t\tif e.Seq != n {", "\t\tif false {", "./internal/journal/"),
    ("journal: chain link not checked", "internal/journal/journal.go", "\t\tif e.Prev != hex.EncodeToString(prev[:]) {", "\t\tif false {", "./internal/journal/"),
    ("journal: secret keys allowed", "internal/journal/journal.go", "\t\tif slices.Contains(secretKeys, strings.ToLower(k)) {", "\t\tif slices.Contains(secretKeys, strings.ToLower(k)) && false {", "./internal/journal/"),
    ("polkit: any exit approves", "internal/polkit/polkit.go", '\t\treturn false, fmt.Errorf("polkit: not approved: %w", err)', "\t\treturn true, nil", "./internal/polkit/"),
    ("daemon: approve skips polkit", "internal/daemon/daemon.go", "\tapproved, err := dm.d.Polkit.Check(ctx, r, dm.lift(text))", "\tapproved, err := true, error(nil)\n\t_, _ = ctx, text", "./internal/daemon/"),
    ("daemon: uid not checked", "internal/daemon/daemon.go", "\tif !slices.Contains(dm.cfg.AllowedUIDs, r.UID) {", "\tif false {", "./internal/daemon/"),
    ("daemon: binary not re-checked", "internal/daemon/daemon.go", "\tif err := dm.d.Unchanged(cd.Path, cd.BinarySHA256); err != nil {", "\tif err := error(nil); err != nil {", "./internal/daemon/"),
    ("daemon: journal errors ignored", "internal/daemon/daemon.go", "\treturn dm.d.Journal.Append(kind, id, h, detail)", "\tdm.d.Journal.Append(kind, id, h, detail)\n\treturn nil", "./internal/daemon/"),
    ("daemon: desk refusal rejects with phone on", "internal/daemon/daemon.go", "\t\tcase dm.d.Phone == nil && ctx.Err() == nil:", "\t\tcase ctx.Err() == nil:", "./internal/daemon/"),
    ("daemon: broken rules ignored", "internal/daemon/daemon.go", '\t\trefuse("", none, "the rules file cannot be read, so nothing runs: "+err.Error())\n\t\treturn', '\t\t_ = err', "./internal/daemon/"),
    ("resolve: writable files trusted", "internal/resolve/resolve.go", "\t\tif fi.Mode().Perm()&0o022 != 0 {", "\t\tif false {", "./internal/resolve/"),
    ("runner: requester env passed", "internal/runner/runner.go", "\t\tEnv:         Env(),", "\t\tEnv:         os.Environ(),", "./internal/runner/"),
    ("phone: any chat heard", "internal/phone/phone.go", '\treturn m != nil && m.Chat.ID == p.bot.chat && m.Chat.Type == "private"', "\treturn m != nil", "./internal/phone/"),
    ("phone: secret message kept", "internal/phone/phone.go", "\tp.bot.Delete(ctx, m.ID)\n", "\t_ = m.ID\n", "./internal/phone/"),
    ("phone: stale card hash accepted", "internal/phone/phone.go", "\tif o == nil || parts[2] != hash8(o.rec) {", "\tif o == nil {", "./internal/phone/"),
    ("daemon: dialog opens in review mode", "internal/daemon/daemon.go", "\tif dm.cfg.DeskDialog {", "\tif true {", "./internal/daemon/"),
    ("daemon: card the dialog cannot show is held", "internal/daemon/daemon.go", "\tif err != nil {\n\t\trefuse(id, cd.Hash, err.Error()+\": ask again", "\tif err != nil && false {\n\t\trefuse(id, cd.Hash, err.Error()+\": ask again", "./internal/daemon/"),
    ("daemon: approve takes another user's card", "internal/daemon/daemon.go", "\tif !ok || rec.Status != card.Held || rec.Card.Req.UID != r.UID {", "\tif !ok || rec.Status != card.Held {", "./internal/daemon/"),
    ("daemon: dismissed approval keeps the card", "internal/daemon/daemon.go", "\t\tif !errors.Is(err, polkit.ErrNoAgent) && ctx.Err() == nil {", "\t\tif false {", "./internal/daemon/"),
    ("daemon: approve with no agent rejects", "internal/daemon/daemon.go", "\t\tif !errors.Is(err, polkit.ErrNoAgent) && ctx.Err() == nil {", "\t\tif ctx.Err() == nil {", "./internal/daemon/"),
    ("daemon: card not lifted on Omarchy", "internal/daemon/daemon.go", "\tif !dm.cfg.DialogLift {", "\tif true {", "./internal/daemon/"),
    ("card: dialog shows a card that does not fit", "internal/card/render.go", "\tif !fits || len(lines) > DialogLines {", "\tif false {", "./internal/card/"),
    ("card: dialog ignores its line count", "internal/card/render.go", "\tif !fits || len(lines) > DialogLines {", "\tif !fits {", "./internal/card/"),
    ("card: wide runes shown", "internal/card/render.go", "\tcase r >= 0xc0 && r <= 0x24f:", "\tcase r >= 0xc0:", "./internal/card/"),
    ("card: agent text unmarked", "internal/card/render.go", "\t\t{\"Agent says: \", agentMore,", "\t\t{\"Agent says: \", \"  \",", "./internal/card/"),
    ("gates: one go build may drop -bindnow", "internal/gates/check.go", "\t\tif !strings.Contains(line, \"-bindnow\") {", "\t\tif false {", "./internal/gates/"),
    ("gates: a binary need not be PIE", "internal/gates/check.go", "\tif f.Type != elf.ET_DYN || !has(elf.DT_FLAGS_1, uint64(elf.DF_1_PIE)) {", "\tif false {", "./internal/gates/"),
    ("gates: the first build() is read", "internal/gates/check.go", "\ti := strings.LastIndex(src, head)", "\ti := strings.Index(src, head)", "./internal/gates/"),
    ("card: dialog lines never wrap", "internal/card/render.go", "\t\tif utf8.RuneCountInString(line)+1+utf8.RuneCountInString(t) <= DialogWidth {", "\t\tif true {", "./internal/card/"),
    ("card: token wider than a line accepted", "internal/card/render.go", "\t\tok = ok && utf8.RuneCountInString(l) <= DialogWidth", "\t\t_ = l", "./internal/card/"),
    ("oncerootd: no lift on Omarchy", "cmd/oncerootd/main.go", "\treturn onOmarchy, onOmarchy", "\treturn onOmarchy, false", "./cmd/oncerootd/"),
    ("review: a failed action reads as done", "cmd/onceroot/bar.go", "\t\t\tfmt.Fprintf(out, \"%s %s failed: %v\\n\\n\", op, c.ID, err)\n\t\t\tcontinue", "\t\t\t_ = err", "./cmd/onceroot/"),
    ("card: empty risk label not guarded", "internal/card/render.go", "\tif len(tokens) == 0 {\n\t\ttokens = []string{\"\"}\n\t}\n", "", "./internal/card/"),
    ("polkit: no agent read as a refusal", "internal/polkit/polkit.go", "\tif errors.As(err, &exit) && exit.ExitCode() == 2 {", "\tif errors.As(err, &exit) && exit.ExitCode() == 99 {", "./internal/polkit/"),
    ("daemon: no desk agent rejects the card", "internal/daemon/daemon.go", "\t\t\tcase errors.Is(err, polkit.ErrNoAgent):", "\t\t\tcase errors.Is(err, polkit.ErrNoAgent) && false:", "./internal/daemon/"),
    ("daemon: approve answers before the journal", "internal/daemon/daemon.go", "\tif err := awaitLogged(logged, rec.Card.Expires); err != nil {\n\t\tev.send(wire.Event{Ev: \"error\", Reason: \"approved, but", "\tif err := awaitLogged(logged, rec.Card.Expires); err != nil && false {\n\t\tev.send(wire.Event{Ev: \"error\", Reason: \"approved, but", "./internal/daemon/"),
    ("daemon: reject answers before the journal", "internal/daemon/daemon.go", "\tif err := awaitLogged(logged, rec.Card.Expires); err != nil {\n\t\tev.send(wire.Event{Ev: \"error\", Reason: \"rejected, but", "\tif err := awaitLogged(logged, rec.Card.Expires); err != nil && false {\n\t\tev.send(wire.Event{Ev: \"error\", Reason: \"rejected, but", "./internal/daemon/"),
    ("daemon: no dialog not journalled", "internal/daemon/daemon.go", "\t\t\t\tdm.journal(\"no_dialog\", id, cd.Hash, nil)\n", "", "./internal/daemon/"),
    ("mcp: a cancelled id stays taken", "internal/mcp/mcp.go", "\t\t\tdelete(cancelled, key) // the id is free again for a later call\n", "", "./internal/mcp/"),
    ("bar: idle prints its JSON", "cmd/onceroot/bar.go", "\tif len(held) == 0 {\n\t\treturn \"\"\n\t}", "", "./cmd/onceroot/"),
    ("daemon: list shows every uid", "internal/daemon/daemon.go", "\t\tif rec.Card.Req.UID != uid {\n\t\t\tcontinue", "\t\tif false {\n\t\t\tcontinue", "./internal/daemon/"),
    ("daemon: reject ignores the uid", "internal/daemon/daemon.go", "\tif !ok || rec.Card.Req.UID != r.UID {", "\tif !ok {", "./internal/daemon/"),
    ("mcp: cancellation ignored", "internal/mcp/mcp.go", "\t\t\t\t\tcancelled[string(p.RequestID)] = true\n\t\t\t\t\tcancel()", "\t\t\t\t\t_ = cancel", "./internal/mcp/"),
    ("wire: NUL bytes accepted", "internal/wire/wire.go", "\t\t\tif strings.IndexByte(a, 0) >= 0 {", "\t\t\tif false {", "./internal/wire/"),
]


def copy_repo(root, dst):
    files = subprocess.run(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
                           cwd=root, check=True, capture_output=True).stdout.split(b"\0")
    for f in files:
        if not f:
            continue
        rel = f.decode()
        os.makedirs(os.path.join(dst, os.path.dirname(rel)), exist_ok=True)
        shutil.copy2(os.path.join(root, rel), os.path.join(dst, rel))


def main():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    caught = survived = invalid = 0
    for name, path, old, new, pkg in MUTANTS:
        with tempfile.TemporaryDirectory(prefix="onceroot-mut-") as dst:
            copy_repo(root, dst)
            p = os.path.join(dst, path)
            src = open(p).read()
            if src.count(old) != 1:
                print(f"INVALID   {name}: the text to mutate appears {src.count(old)} times in {path}")
                invalid += 1
                continue
            open(p, "w").write(src.replace(old, new, 1))
            r = subprocess.run(["go", "test", "-count=1", pkg], cwd=dst, capture_output=True, text=True)
            out = r.stdout + r.stderr
            if "[build failed]" in out or "[setup failed]" in out:
                print(f"INVALID   {name}: does not compile")
                invalid += 1
            elif r.returncode == 0:
                print(f"SURVIVED  {name}")
                survived += 1
            else:
                tests = sorted(set(re.findall(r"--- FAIL: (\w+)", out)))
                print(f"caught    {name}: {', '.join(tests) or 'package failed'}")
                caught += 1
    print(f"\n{caught} caught, {survived} survived, {invalid} invalid, of {len(MUTANTS)}")
    sys.exit(1 if survived or invalid else 0)


if __name__ == "__main__":
    main()
