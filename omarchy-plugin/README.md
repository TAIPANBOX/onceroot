# onceroot for Omarchy's bar

A bar widget for [onceroot](https://github.com/TAIPANBOX/onceroot): root for an
AI coding agent, one approved command at a time.

![The widget's card under the padlock: the whole request, Approve and Reject, and Review in a terminal](preview.png)

- While no request waits, it takes no room on the bar and runs nothing: it
  watches oncerootd's journal, which changes on every request, decision and run.
- While requests wait, a padlock and the count show, highlighted. A click opens
  each card on a solid background, with Approve and Reject.
- Approve hands the card's id to `onceroot approve`, so Omarchy's own
  authentication dialog opens with the same card and decides. The widget cannot
  approve anything by itself: Omarchy gives third-party plugins no
  authentication services, and oncerootd takes approval only through polkit.
- A right click opens `onceroot review` in Omarchy's floating terminal.
- If oncerootd cannot be reached, the padlock shows a question mark.
- English and Ukrainian.

## Requirements

- Omarchy 4.5 or later.
- onceroot 0.1.0 or later, installed and running, with your user in the
  `onceroot` group: [install onceroot](https://github.com/TAIPANBOX/onceroot#install).
  The widget calls only the `onceroot` command; it has no other dependency.

## Install

```sh
omarchy plugin add https://github.com/TAIPANBOX/onceroot-omarchy.git --enable
```

The onceroot package also carries this widget, in
`/usr/share/onceroot/omarchy-plugin`:

```sh
cp -r /usr/share/onceroot/omarchy-plugin ~/.config/omarchy/plugins/taipanbox.onceroot
omarchy bar put taipanbox.onceroot --section right
```

## Remove

```sh
omarchy plugin remove taipanbox.onceroot
```

This takes the widget off the bar and deletes its folder. onceroot itself stays
installed; remove it with `sudo pacman -R onceroot`.

## Source and checks

This repository is published from
[onceroot's omarchy-plugin folder](https://github.com/TAIPANBOX/onceroot/tree/main/omarchy-plugin),
where the widget's tests and gates run in CI: every text is plain text, commands
never go through a shell string, and the id of a card is checked before anything
runs.

## License

MIT, see [LICENSE](LICENSE).
