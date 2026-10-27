#!/bin/sh
# Builds the onceroot package from this checkout with makepkg: the same
# PKGBUILD a release uses, with its source switched to a tarball of HEAD (so
# commit what you want packaged first). check() runs the tests and measures
# the binaries. Run it as a normal user; extra arguments go to makepkg.
set -eu
cd "$(dirname "$0")/.."
ver=$(sed -n 's/^pkgver=//p' packaging/PKGBUILD)
work=$(mktemp -d "${TMPDIR:-/tmp}/onceroot-pkg.XXXXXX")
trap 'rm -rf "$work"' EXIT
git archive --prefix="onceroot-$ver/" -o "$work/onceroot-$ver.tar.gz" HEAD
sum=$(sha256sum "$work/onceroot-$ver.tar.gz" | cut -d' ' -f1)
sed -e "s|^source=.*|source=(\"onceroot-$ver.tar.gz\")|" \
    -e "s|^sha256sums=.*|sha256sums=('$sum')|" packaging/PKGBUILD > "$work/PKGBUILD"
(cd "$work" && makepkg -f "$@")
cp "$work"/onceroot-*.pkg.tar.zst .
ls onceroot-*.pkg.tar.zst
