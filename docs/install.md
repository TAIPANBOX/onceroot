# Installing onceroot

## What you need

- Arch Linux or Omarchy, with systemd and polkit (both are there by default).
- To build: `git`, `go` (1.27 or later) and `base-devel`.
- A polkit authentication agent in your desktop session. Omarchy's shell has
  one. Without one, a card waits for `onceroot review` or the phone.

## With the package (recommended)

```sh
git clone https://github.com/TAIPANBOX/onceroot && cd onceroot
scripts/package.sh
sudo pacman -U onceroot-0.1.0-1-*.pkg.tar.zst
```

`scripts/package.sh` runs `makepkg` on [packaging/PKGBUILD](../packaging/PKGBUILD),
the file a release uses, with its source switched to a tarball of `HEAD`. So
commit anything you want packaged first. `check()` runs the whole test suite
and then measures the two binaries it built: position independent, bound at
load time, a RELRO segment, symbols kept for the debug package. Extra arguments
go to `makepkg`.

The package puts:

| Path | What |
|---|---|
| `/usr/bin/onceroot`, `/usr/bin/oncerootd` | the CLI and the service |
| `/usr/lib/systemd/system/oncerootd.service` | the unit: root, `UMask=0022`, `ProtectHome=read-only`, `PrivateTmp=yes` |
| `/usr/share/polkit-1/actions/org.taipanbox.onceroot.policy` | the one polkit action, `auth_admin` for every subject |
| `/usr/lib/sysusers.d/onceroot.conf`, `/usr/lib/tmpfiles.d/onceroot.conf` | the `onceroot` group, `/run/onceroot` and `/var/lib/onceroot` (0750 root:onceroot) |
| `/etc/onceroot/onceroot.conf`, `/etc/onceroot/rules` | settings and rules, kept on upgrade |
| `/usr/share/onceroot/skill/onceroot/SKILL.md` | the skill for agents |

pacman's own hooks create the group and the folders when the package is
installed.

## From a release

Each release carries the package, the binaries and `SHA256SUMS`, every file with
a build provenance attestation from this repository's release workflow:

```sh
gh release download v0.1.0 --repo TAIPANBOX/onceroot -p 'onceroot-*.pkg.tar.zst'
gh attestation verify onceroot-0.1.0-1-x86_64.pkg.tar.zst --repo TAIPANBOX/onceroot
sudo pacman -U onceroot-0.1.0-1-x86_64.pkg.tar.zst
```

Or build the released version yourself: `cd packaging && makepkg -si` fetches the
tagged source from GitHub and checks it against the checksum in the PKGBUILD.

## Then

1. Put your uid (`id -u`) into `allowed_uids` in `/etc/onceroot/onceroot.conf`.
2. `sudo usermod -aG onceroot "$USER"`, and log in again: the socket is
   `0660 root:onceroot`.
3. `sudo systemctl enable --now oncerootd`, then `onceroot status`.

## The settings file

`/etc/onceroot/onceroot.conf` must belong to root and be writable by root only;
otherwise `oncerootd` does not start. A line it does not know stops it too.

| Setting | Default | Meaning |
|---|---|---|
| `allowed_uids` | none | users whose agents may ask, comma-separated |
| `ttl` | `10m` | how long a card waits for a person |
| `max_open` | `3` | open cards per user |
| `max_per_hour` | `30` | requests per user per hour |
| `pause_after` | `3` | rejections in a row before a pause |
| `pause` | `15m` | how long the pause lasts |
| `command_limit` | `30m` | time limit of one command |
| `desk_prompt` | `auto` | `auto`: the dialog opens at once on Omarchy, elsewhere approve from `onceroot review`; `dialog`: always open it; `review`: never open it on its own |
| `phone` | off | `telegram` turns on approval from the phone ([phone.md](phone.md)) |
| `telegram_chat` | none | the one private chat the phone module answers |

The rules file is described in [rules.md](rules.md). It is read on every
request, so an edit applies at once.

## Without the package

From a checkout, as root, the same files by hand:

```sh
go build -o onceroot ./cmd/onceroot && go build -o oncerootd ./cmd/oncerootd
sudo install -Dm755 onceroot oncerootd -t /usr/bin
sudo install -Dm644 packaging/oncerootd.service /usr/lib/systemd/system/oncerootd.service
sudo install -Dm644 packaging/org.taipanbox.onceroot.policy /usr/share/polkit-1/actions/org.taipanbox.onceroot.policy
sudo install -Dm644 packaging/onceroot.sysusers /usr/lib/sysusers.d/onceroot.conf
sudo install -Dm644 packaging/onceroot.tmpfiles /usr/lib/tmpfiles.d/onceroot.conf
sudo install -Dm644 packaging/onceroot.conf /etc/onceroot/onceroot.conf
sudo install -Dm644 packaging/rules.example /etc/onceroot/rules
sudo systemd-sysusers && sudo systemd-tmpfiles --create
```

Nothing then removes these files for you, which is why the package is the
recommended way.

## Upgrading and removing

Pull, run `scripts/package.sh` again, `sudo pacman -U` the new package and
`sudo systemctl restart oncerootd`. Your settings and rules stay; an upgraded
default arrives as a `.pacnew` file.

To remove it: `sudo systemctl disable --now oncerootd && sudo pacman -R onceroot`.
The journal and its key in `/var/lib/onceroot` stay until you delete them.
