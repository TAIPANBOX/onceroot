package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseReadsEveryKey(t *testing.T) {
	f, err := Parse(strings.NewReader(`# onceroot
allowed_uids = 1000, 1001
ttl = 5m
max_open = 2
max_per_hour = 10
pause_after = 4
pause = 20m
command_limit = 1h
phone = telegram
telegram_chat = 123456789
`))
	if err != nil {
		t.Fatal(err)
	}
	d := f.Daemon
	if len(d.AllowedUIDs) != 2 || d.AllowedUIDs[1] != 1001 || d.Limits.TTL != 5*time.Minute || d.Limits.MaxOpen != 2 ||
		d.Limits.MaxPerHour != 10 || d.Limits.PauseAfter != 4 || d.Limits.Pause != 20*time.Minute || d.CommandLimit != time.Hour ||
		f.Phone != "telegram" || f.TelegramChat != 123456789 {
		t.Fatalf("got %+v", f)
	}
}

func TestParseFillsDefaults(t *testing.T) {
	f, err := Parse(strings.NewReader("allowed_uids = 1000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Daemon.Limits.TTL != 10*time.Minute || f.Daemon.CommandLimit != 30*time.Minute || f.Phone != "off" ||
		f.Daemon.MaxCardRunes != 4000 || f.Daemon.RootPATH != "/usr/local/sbin:/usr/local/bin:/usr/bin" || f.Daemon.SudoersDir != "/etc/sudoers.d" {
		t.Fatalf("defaults %+v", f)
	}
}

func TestParseRefusesBadConfig(t *testing.T) {
	for name, text := range map[string]string{
		"unknown key":         "allowed_uids = 1000\ncolour = red\n",
		"no uids":             "ttl = 5m\n",
		"root as a user":      "allowed_uids = 0\n",
		"bad duration":        "allowed_uids = 1000\nttl = soon\n",
		"zero limit":          "allowed_uids = 1000\nmax_open = 0\n",
		"phone without chat":  "allowed_uids = 1000\nphone = telegram\n",
		"unknown phone":       "allowed_uids = 1000\nphone = sms\n",
		"line without equals": "allowed_uids 1000\n",
		"duplicate key":       "allowed_uids = 1000\nallowed_uids = 1001\n",
		"ttl over a day":      "allowed_uids = 1000\nttl = 48h\n",
	} {
		if _, err := Parse(strings.NewReader(text)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestDeskPromptValues(t *testing.T) {
	f, err := Parse(strings.NewReader("allowed_uids = 1000\n"))
	if err != nil || f.DeskPrompt != "auto" {
		t.Fatalf("default %q, %v", f.DeskPrompt, err)
	}
	for _, v := range []string{"auto", "dialog", "review"} {
		f, err := Parse(strings.NewReader("allowed_uids = 1000\ndesk_prompt = " + v + "\n"))
		if err != nil || f.DeskPrompt != v {
			t.Errorf("%s: got %q, %v", v, f.DeskPrompt, err)
		}
	}
	if _, err := Parse(strings.NewReader("allowed_uids = 1000\ndesk_prompt = popup\n")); err == nil {
		t.Error("desk_prompt = popup accepted")
	}
}
