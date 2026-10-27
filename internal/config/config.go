// Package config reads /etc/onceroot/onceroot.conf: one "key = value" per line,
// "#" comments. Unknown keys, duplicates and values out of range are refused,
// so a typo never silently falls back to a default.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/daemon"
	"github.com/TAIPANBOX/onceroot/internal/resolve"
	"github.com/TAIPANBOX/onceroot/internal/store"
)

// File is the whole config.
type File struct {
	Daemon       daemon.Config
	Phone        string // off | telegram
	TelegramChat int64
	DeskPrompt   string // auto | dialog | review
}

// Parse reads a config file and fills the defaults of spec 6.3.
func Parse(r io.Reader) (File, error) {
	f := File{
		Daemon: daemon.Config{
			Limits:       store.DefaultLimits(),
			CommandLimit: 30 * time.Minute,
			MaxCardRunes: 4000,
			RootPATH:     resolve.RootPATH,
			SudoersDir:   "/etc/sudoers.d",
		},
		Phone:      "off",
		DeskPrompt: "auto",
	}
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return File{}, fmt.Errorf("config: line %d: want key = value", n)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if seen[key] {
			return File{}, fmt.Errorf("config: line %d: %s set twice", n, key)
		}
		seen[key] = true
		if err := f.set(key, val); err != nil {
			return File{}, fmt.Errorf("config: line %d: %s: %w", n, key, err)
		}
	}
	if err := sc.Err(); err != nil {
		return File{}, fmt.Errorf("config: %w", err)
	}
	if len(f.Daemon.AllowedUIDs) == 0 {
		return File{}, errors.New("config: allowed_uids is required")
	}
	if f.Phone == "telegram" && f.TelegramChat == 0 {
		return File{}, errors.New("config: phone = telegram needs telegram_chat")
	}
	return f, nil
}

func (f *File) set(key, val string) error {
	d := &f.Daemon
	switch key {
	case "allowed_uids":
		for _, s := range strings.Split(val, ",") {
			u, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
			if err != nil {
				return err
			}
			if u == 0 {
				return errors.New("root does not need onceroot")
			}
			d.AllowedUIDs = append(d.AllowedUIDs, uint32(u))
		}
	case "ttl":
		return duration(val, time.Minute/2, 24*time.Hour, &d.Limits.TTL)
	case "pause":
		return duration(val, time.Minute, 24*time.Hour, &d.Limits.Pause)
	case "command_limit":
		return duration(val, time.Second, 24*time.Hour, &d.CommandLimit)
	case "max_open":
		return count(val, &d.Limits.MaxOpen)
	case "max_per_hour":
		return count(val, &d.Limits.MaxPerHour)
	case "pause_after":
		return count(val, &d.Limits.PauseAfter)
	case "phone":
		if val != "off" && val != "telegram" {
			return errors.New("want off or telegram")
		}
		f.Phone = val
	case "desk_prompt":
		if val != "auto" && val != "dialog" && val != "review" {
			return errors.New("want auto, dialog or review")
		}
		f.DeskPrompt = val
	case "telegram_chat":
		id, err := strconv.ParseInt(val, 10, 64)
		if err != nil || id <= 0 {
			return errors.New("want the private chat's positive numeric id")
		}
		f.TelegramChat = id
	default:
		return errors.New("unknown key")
	}
	return nil
}

func duration(val string, lo, hi time.Duration, out *time.Duration) error {
	d, err := time.ParseDuration(val)
	if err != nil {
		return err
	}
	if d < lo || d > hi {
		return fmt.Errorf("must be between %s and %s", lo, hi)
	}
	*out = d
	return nil
}

func count(val string, out *int) error {
	n, err := strconv.Atoi(val)
	if err != nil {
		return err
	}
	if n < 1 || n > 1000 {
		return errors.New("must be between 1 and 1000")
	}
	*out = n
	return nil
}
