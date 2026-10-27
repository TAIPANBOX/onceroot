// Command oncerootd is onceroot's root daemon. It reads
// /etc/onceroot/onceroot.conf and /etc/onceroot/rules, journals to
// /var/lib/onceroot and listens on /run/onceroot/onceroot.sock.
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TAIPANBOX/onceroot/internal/config"
	"github.com/TAIPANBOX/onceroot/internal/daemon"
	"github.com/TAIPANBOX/onceroot/internal/factor"
	"github.com/TAIPANBOX/onceroot/internal/journal"
	"github.com/TAIPANBOX/onceroot/internal/peer"
	"github.com/TAIPANBOX/onceroot/internal/phone"
	"github.com/TAIPANBOX/onceroot/internal/polkit"
	"github.com/TAIPANBOX/onceroot/internal/resolve"
	"github.com/TAIPANBOX/onceroot/internal/rules"
	"github.com/TAIPANBOX/onceroot/internal/runner"
	"github.com/TAIPANBOX/onceroot/internal/store"
)

// layout is where oncerootd finds and keeps everything. Tests point it at a
// temporary tree; oncerootd uses defaultLayout.
type layout struct {
	config, rules, state, sock, token, factors, pkcheck, omarchy, group string
	owner                                                               uint32 // owner the config and rules must have: root
}

var defaultLayout = layout{
	config:  "/etc/onceroot/onceroot.conf",
	rules:   "/etc/onceroot/rules",
	state:   "/var/lib/onceroot",
	sock:    "/run/onceroot/onceroot.sock",
	token:   "/etc/onceroot/telegram.token",
	factors: "/etc/onceroot/factors",
	pkcheck: polkit.DefaultBin,
	omarchy: "/usr/share/omarchy",
	group:   "onceroot",
	owner:   0,
}

func main() {
	l := defaultLayout
	flag.StringVar(&l.config, "config", l.config, "config file")
	flag.Parse()
	var err error
	switch flag.Arg(0) {
	case "setup-phone":
		if os.Geteuid() != 0 {
			err = errors.New("must run as root")
			break
		}
		host, _ := os.Hostname()
		err = setupPhone(os.Stdin, os.Stdout, l.factors, l.state+"/totp.used", host)
	case "":
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		done := make(chan struct{})
		go func() { <-stop; close(done) }()
		err = serve(l, done)
	default:
		err = fmt.Errorf("unknown command %q", flag.Arg(0))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "oncerootd:", err)
		os.Exit(1)
	}
}

// serve runs the daemon until done closes.
func serve(l layout, done <-chan struct{}) error {
	if os.Geteuid() != 0 {
		return errors.New("must run as root")
	}
	if err := rootOnly(l.config, l.owner); err != nil {
		return err
	}
	f, err := os.Open(l.config)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(f)
	f.Close()
	if err != nil {
		return err
	}
	cfg.Daemon.DeskDialog, cfg.Daemon.DialogLift = desk(cfg.DeskPrompt, l.omarchy)
	gid, err := groupID(l.group)
	if err != nil {
		return err
	}
	key, err := journal.LoadKey(l.state + "/journal.key")
	if err != nil {
		return err
	}
	if err := journal.WritePublicKey(l.state+"/journal.pub", key.Public().(ed25519.PublicKey)); err != nil {
		return err
	}
	j, err := journal.Open(l.state+"/journal", key)
	if err != nil {
		return err
	}
	defer j.Close()
	if err := os.Chown(l.state+"/journal", 0, gid); err != nil {
		return err
	}
	st := store.NewStore(cfg.Daemon.Limits, time.Now)
	deps := daemon.Deps{
		Peer: peer.FromConn, StillSame: peer.StillSame,
		Resolve: resolve.Resolve, Unchanged: resolve.Unchanged,
		Rules:  func() ([]rules.Rule, error) { return loadRules(l.rules, l.owner) },
		Polkit: polkit.PK{Bin: l.pkcheck}, Run: runner.Run,
		Journal: j, Store: st, Clock: time.Now,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.Phone == "telegram" {
		token, factors, err := phone.Load(l.token, l.factors, l.state+"/totp.used")
		if err != nil {
			return err
		}
		ph := phone.New(phone.NewBot(phone.BaseURL, token, cfg.TelegramChat, nil), factors, time.Now)
		go ph.Run(ctx)
		deps.Phone = ph
	}
	d := daemon.New(cfg.Daemon, deps)

	os.Remove(l.sock)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: l.sock, Net: "unix"})
	if err != nil {
		return err
	}
	if err := os.Chown(l.sock, 0, gid); err != nil {
		ln.Close()
		return err
	}
	if err := os.Chmod(l.sock, 0o660); err != nil {
		ln.Close()
		return err
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				st.Sweep()
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { <-done; ln.Close() }()
	err = d.Serve(ln)
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// loadRules reads the rules file on every request, so an edit applies at
// once. A missing file means no rules: every request asks a person. A file
// others can write, one of another owner, or one that does not parse refuses
// every request.
func loadRules(path string, owner uint32) ([]rules.Rule, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat() // the file read, not whatever the path names later
	if err != nil {
		return nil, err
	}
	if err := ownedByRoot(path, fi, owner); err != nil {
		return nil, err
	}
	return rules.Parse(f)
}

// rootOnly requires a file owned by owner (root) and writable by nobody else.
func rootOnly(path string, owner uint32) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	return ownedByRoot(path, fi, owner)
}

func ownedByRoot(path string, fi os.FileInfo, owner uint32) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != owner || fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must belong to root and be writable by root only", path)
	}
	return nil
}

func groupID(name string) (int, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, fmt.Errorf("group %s: %w", name, err)
	}
	return strconv.Atoi(g.Gid)
}

// setupPhone writes the factors file from a PIN read on in and prints the
// otpauth link for an authenticator app, once. Feed the PIN without echo:
// read -rs PIN; echo "$PIN" | sudo oncerootd setup-phone
func setupPhone(in io.Reader, out io.Writer, factorsPath, usedPath, host string) error {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return errors.New("give the PIN on stdin")
	}
	secret, err := factor.NewSecret()
	if err != nil {
		return err
	}
	if err := phone.WriteFactors(factorsPath, strings.TrimSpace(line), secret); err != nil {
		return err
	}
	if err := os.Remove(usedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Fprintln(out, "Add this to your authenticator app, then keep it nowhere else:")
	fmt.Fprintln(out, factor.URI(secret, "onceroot@"+host, "onceroot"))
	return nil
}

// desk resolves desk_prompt and the dialog's layout. auto opens the polkit
// dialog at once only on Omarchy, whose polkit agent shows the card it is
// given; elsewhere the person approves from onceroot review, where the whole
// card is on screen. lift is on wherever Omarchy's dialog shows the card,
// which centres it on a band just above the password field.
func desk(mode, omarchyDir string) (dialog, lift bool) {
	fi, err := os.Stat(omarchyDir)
	onOmarchy := err == nil && fi.IsDir()
	switch mode {
	case "dialog":
		return true, onOmarchy
	case "review":
		return false, onOmarchy
	}
	return onOmarchy, onOmarchy
}
