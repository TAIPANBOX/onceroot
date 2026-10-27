package rules

import (
	"slices"
	"strings"
	"testing"
)

func mustParse(t *testing.T, text string) []Rule {
	t.Helper()
	rs, err := Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestParseRejectsBadLines(t *testing.T) {
	cases := map[string]string{
		"unknown verb":         "permit /usr/bin/true",
		"no path":              "allow",
		"relative path":        "allow usr/bin/true",
		"star inside pattern":  "allow /usr/bin/pacman -*Q",
		"star in path":         "allow /usr/bin/pac*",
		"ellipsis not last":    "allow /usr/bin/pacman ... -Q",
		"control char in line": "allow /usr/bin/true \x01",
	}
	for name, line := range cases {
		_, err := Parse(strings.NewReader("# ok\n\n" + line + "\n"))
		if err == nil {
			t.Errorf("%s: %q accepted", name, line)
			continue
		}
		if !strings.Contains(err.Error(), "line 3") {
			t.Errorf("%s: error does not name line 3: %v", name, err)
		}
	}
}

func TestParseSkipsCommentsAndBlanks(t *testing.T) {
	rs := mustParse(t, "# comment\n\n   \nallow /usr/bin/true  # trailing\n")
	if len(rs) != 1 || rs[0].Line != 4 {
		t.Fatalf("got %+v", rs)
	}
}

func TestFirstMatchWins(t *testing.T) {
	rs := mustParse(t, "deny /usr/bin/pacman -R* ...\nallow /usr/bin/pacman ...\n")
	if v, _ := Decide(rs, "/usr/bin/pacman", []string{"pacman", "-Rns", "foo"}); v != Deny {
		t.Fatalf("deny listed first: got %v", v)
	}
	if v, _ := Decide(rs, "/usr/bin/pacman", []string{"pacman", "-Qi", "foo"}); v != Allow {
		t.Fatalf("allow listed second: got %v", v)
	}
}

func TestNoMatchMeansAsk(t *testing.T) {
	rs := mustParse(t, "allow /usr/bin/pacman -Q* ...\n")
	v, reason := Decide(rs, "/usr/bin/systemctl", []string{"systemctl", "restart", "sshd"})
	if v != Ask || reason == "" {
		t.Fatalf("got %v %q, want Ask with a reason", v, reason)
	}
}

func TestArgCountMustMatchWithoutEllipsis(t *testing.T) {
	rs := mustParse(t, "allow /usr/bin/systemctl status sshd\n")
	if v, _ := Decide(rs, "/usr/bin/systemctl", []string{"systemctl", "status", "sshd"}); v != Allow {
		t.Fatalf("exact match: got %v", v)
	}
	if v, _ := Decide(rs, "/usr/bin/systemctl", []string{"systemctl", "status", "sshd", "--no-pager"}); v != Ask {
		t.Fatalf("an extra argument slipped through: got %v", v)
	}
	if v, _ := Decide(rs, "/usr/bin/systemctl", []string{"systemctl", "status"}); v != Ask {
		t.Fatalf("a missing argument matched: got %v", v)
	}
}

func TestPrefixPattern(t *testing.T) {
	rs := mustParse(t, "allow /usr/bin/pacman -Q* ...\n")
	for _, ok := range [][]string{{"pacman", "-Q"}, {"pacman", "-Qi", "ripgrep"}} {
		if v, _ := Decide(rs, "/usr/bin/pacman", ok); v != Allow {
			t.Errorf("%v: got %v, want Allow", ok, v)
		}
	}
	if v, _ := Decide(rs, "/usr/bin/pacman", []string{"pacman", "-S", "ripgrep"}); v != Ask {
		t.Fatalf("-S matched -Q*: got %v", v)
	}
}

func TestRuleMatchesTheResolvedPathOnly(t *testing.T) {
	rs := mustParse(t, "allow /usr/bin/pacman ...\n")
	if v, _ := Decide(rs, "/usr/local/bin/pacman", []string{"pacman", "-Q"}); v != Ask {
		t.Fatalf("another binary with the same name matched: got %v", v)
	}
}

func TestBuiltinDenyBeatsAllowRule(t *testing.T) {
	rs := mustParse(t, "allow /usr/bin/bash ...\nallow /usr/bin/env ...\n")
	for _, argv := range [][]string{{"bash", "-c", "id"}, {"bash", "script.sh"}, {"env", "pacman", "-Q"}} {
		path := "/usr/bin/" + argv[0]
		v, reason := Decide(rs, path, argv)
		if v != Deny || !strings.HasPrefix(reason, "built-in") {
			t.Errorf("%v: got %v %q, want a built-in Deny", argv, v, reason)
		}
	}
}

func TestBuiltinDeniesWhatTheCardCannotShow(t *testing.T) {
	denied := []string{
		"sh", "bash", "zsh", "fish", "dash", "busybox",
		"python3", "perl", "ruby", "node", "php", "lua",
		"env", "xargs", "nice", "nohup", "timeout", "setsid", "chroot", "unshare", "nsenter", "systemd-run", "script", "watch", "flock", "stdbuf",
		"sudo", "su", "pkexec", "doas", "run0", "sudoedit", "visudo", "runuser", "setpriv", "capsh",
		"vi", "vim", "nvim", "nano", "emacs",
	}
	for _, name := range denied {
		if deny, _ := Builtin("/usr/bin/"+name, []string{name, "x"}); !deny {
			t.Errorf("%s is not denied", name)
		}
	}
	if deny, _ := Builtin("/usr/bin/busybox", []string{"sh", "-c", "id"}); !deny {
		t.Error("a symlinked busybox applet is not denied")
	}
	for _, name := range []string{"pacman", "systemctl", "cp", "mkdir"} {
		if deny, why := Builtin("/usr/bin/"+name, []string{name}); deny {
			t.Errorf("%s denied: %s", name, why)
		}
	}
}

func TestRiskLabels(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"pacman", "-U", "/tmp/x.pkg.tar.zst"}, "installs a local package file"},
		{[]string{"pacman", "-S", "--noconfirm", "ripgrep"}, "installs packages"},
		{[]string{"systemctl", "edit", "sshd"}, "opens an editor as root"},
		{[]string{"cp", "x", "/etc/sudoers.d/agent"}, "touches sudo rules"},
		{[]string{"cp", "x", "/etc/polkit-1/rules.d/10.rules"}, "touches polkit rules"},
		{[]string{"chmod", "-R", "777", "/"}, "recursive on /"},
		{[]string{"rm", "-rf", "/"}, "recursive on /"},
		{[]string{"dd", "if=/dev/zero", "of=/dev/sda"}, "writes disks or partitions"},
		{[]string{"mkfs.ext4", "/dev/sdb1"}, "writes disks or partitions"},
		{[]string{"find", "/", "-name", "x", "-delete"}, "find deletes or runs commands"},
		{[]string{"usermod", "-aG", "wheel", "bob"}, "changes accounts"},
		{[]string{"cp", "x", "/usr/bin/ls"}, "writes system files"},
	}
	for _, c := range cases {
		got := Risks("/usr/bin/"+c.argv[0], c.argv)
		if !contains(got, c.want) {
			t.Errorf("%v: risks %v lack %q", c.argv, got, c.want)
		}
	}
	if got := Risks("/usr/bin/pacman", []string{"pacman", "-Qi", "ripgrep"}); len(got) != 0 {
		t.Errorf("a query got risks %v", got)
	}
}

func contains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

func TestVerdictNames(t *testing.T) {
	if Ask.String() != "ask" || Allow.String() != "allow" || Deny.String() != "deny" {
		t.Fatal("verdict names changed")
	}
}

func TestRiskLabelsLeaveReadsAlone(t *testing.T) {
	for _, argv := range [][]string{{"pacman", "-Ss", "rg"}, {"pacman", "-Si", "rg"}, {"pacman", "-Sl"}, {"rm", "x"}, {"chmod", "644", "/etc/x"}} {
		got := Risks("/usr/bin/"+argv[0], argv)
		for _, l := range got {
			if l == "installs packages" || l == "recursive on /" || l == "removes files recursively" {
				t.Errorf("%v labelled %q", argv, l)
			}
		}
	}
	for _, c := range []struct {
		argv []string
		want string
	}{
		{[]string{"pacman", "-Syu"}, "installs packages"},
		{[]string{"pacman", "--sync", "rg"}, "installs packages"},
		{[]string{"pacman", "-Rns", "rg"}, "removes packages"},
		{[]string{"chown", "--recursive", "u", "/"}, "recursive on /"},
		{[]string{"rm", "-r", "/tmp/x"}, "removes files recursively"},
	} {
		if !contains(Risks("/usr/bin/"+c.argv[0], c.argv), c.want) {
			t.Errorf("%v lacks %q", c.argv, c.want)
		}
	}
}
