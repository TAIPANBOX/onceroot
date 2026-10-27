package rules

import (
	"path/filepath"
	"slices"
	"strings"
)

// Risks labels a request for the card. Labels are onceroot's words, never
// the agent's; an empty list reads "none detected", not "safe".
func Risks(path string, argv []string) []string {
	name := filepath.Base(path)
	var args []string
	if len(argv) > 0 {
		args = argv[1:]
	}
	var out []string
	add := func(label string) {
		if !slices.Contains(out, label) {
			out = append(out, label)
		}
	}

	switch {
	case name == "pacman":
		for _, a := range args {
			switch {
			case a == "--upgrade" || pacmanOp(a, 'U'):
				add("installs a local package file")
			case a == "--sync" || (pacmanOp(a, 'S') && !onlyFlags(a[2:], "silgpqw")):
				add("installs packages")
			case a == "--remove" || pacmanOp(a, 'R'):
				add("removes packages")
			}
		}
	case name == "systemctl":
		for _, a := range args {
			if a == "edit" {
				add("opens an editor as root")
			}
		}
	case name == "chmod" || name == "chown" || name == "chgrp" || name == "rm":
		if recursive(args) && touchesRoot(args) {
			add("recursive on /")
		}
		if name == "rm" && recursive(args) {
			add("removes files recursively")
		}
	case name == "dd" || strings.HasPrefix(name, "mkfs") || oneOf(name, "wipefs", "sgdisk", "sfdisk", "fdisk", "gdisk", "parted", "cryptsetup", "mkswap", "blkdiscard"):
		add("writes disks or partitions")
	case name == "find":
		for _, a := range args {
			if oneOf(a, "-exec", "-execdir", "-ok", "-okdir", "-delete") {
				add("find deletes or runs commands")
			}
		}
	case oneOf(name, "usermod", "useradd", "userdel", "passwd", "chpasswd", "groupmod", "groupadd", "groupdel", "gpasswd", "chsh"):
		add("changes accounts")
	}

	for _, a := range args {
		if strings.Contains(a, "/etc/sudoers") {
			add("touches sudo rules")
		}
		if strings.Contains(a, "/etc/polkit-1") || strings.Contains(a, "/usr/share/polkit-1") {
			add("touches polkit rules")
		}
	}
	if oneOf(name, "cp", "mv", "install", "tee", "ln", "rsync", "sed", "truncate", "touch") {
		for _, a := range args {
			if systemPath(a) {
				add("writes system files")
			}
		}
	}
	return out
}

// pacmanOp reports a short-option cluster naming operation op: -S, -Syu.
func pacmanOp(a string, op byte) bool {
	return len(a) >= 2 && a[0] == '-' && a[1] == op
}

func onlyFlags(cluster, allowed string) bool {
	if cluster == "" {
		return false
	}
	for _, r := range cluster {
		if !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

func recursive(args []string) bool {
	for _, a := range args {
		if a == "--recursive" {
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsAny(a[1:], "rR") {
			return true
		}
	}
	return false
}

func touchesRoot(args []string) bool {
	for _, a := range args {
		if a == "/" || a == "/*" {
			return true
		}
	}
	return false
}

func systemPath(a string) bool {
	if i := strings.IndexByte(a, '='); i >= 0 {
		a = a[i+1:]
	}
	for _, p := range []string{"/etc/", "/usr/", "/boot/", "/bin/", "/sbin/", "/lib/", "/lib64/", "/opt/"} {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

func oneOf(s string, set ...string) bool {
	return slices.Contains(set, s)
}
