package rules

import (
	"path/filepath"
	"strings"
)

// The built-in deny list. These programs are denied whatever the rules file
// says (I7), because the card cannot show what they would really do: a
// shell or interpreter runs code the card does not hold (and a script file
// can change after approval), a wrapper runs another command that rules and
// risk labels would never see, a privilege tool hands out root itself, and
// an editor needs a terminal onceroot does not give.
var builtin = map[string]string{}

func init() {
	add := func(reason string, names ...string) {
		for _, n := range names {
			builtin[n] = reason
		}
	}
	add("is a shell or an interpreter; the card cannot show the code it runs",
		"sh", "bash", "zsh", "fish", "dash", "ksh", "mksh", "tcsh", "csh", "busybox", "toybox",
		"python", "pypy", "perl", "ruby", "node", "nodejs", "deno", "bun", "php", "lua", "luajit", "tclsh", "wish", "osascript")
	add("runs another command; ask for that command directly",
		"env", "xargs", "nice", "nohup", "timeout", "setsid", "chroot", "unshare", "nsenter",
		"systemd-run", "script", "watch", "flock", "stdbuf", "ionice", "taskset", "chrt", "time", "strace", "ltrace", "gdb")
	add("hands out privileges itself",
		"sudo", "su", "pkexec", "doas", "run0", "sudoedit", "visudo", "runuser", "setpriv", "capsh", "sg", "newgrp")
	add("is an interactive editor",
		"vi", "vim", "nvim", "nano", "emacs", "ed", "micro", "helix", "hx", "kak")
}

// Builtin reports whether a request is denied by the built-in list. It looks
// at both the resolved program and the name the agent used, so a multi-call
// binary reached through a symlink (busybox as sh) is caught either way.
func Builtin(path string, argv []string) (bool, string) {
	names := []string{filepath.Base(path)}
	if len(argv) > 0 {
		names = append(names, filepath.Base(argv[0]))
	}
	for _, n := range names {
		for _, key := range []string{n, stem(n)} {
			if reason, ok := builtin[key]; ok {
				return true, n + " " + reason
			}
		}
	}
	return false, ""
}

// stem drops a version suffix: python3.12 -> python, lua5.4 -> lua.
func stem(name string) string {
	return strings.TrimRight(name, "0123456789.-")
}
