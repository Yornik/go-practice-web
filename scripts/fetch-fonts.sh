#!/usr/bin/env bash
# Downloads the 3 font families of the site from the npm registry and
# copies the files the site uses to web/static/fonts/. All 3 are OFL 1.1:
# Saira (display: labels, numerals, headings), Atkinson Hyperlegible Next
# (every sentence a person reads) and Atkinson Hyperlegible Mono (code,
# log lines, IDs). The variable builds keep each family in 1 file per
# script range, so a weight or a width costs no extra request.
# The files stay committed; this script documents where they come from
# and reproduces them. Each tarball is checked against the sha512
# integrity that the registry publishes for that version.
#
# Usage: scripts/fetch-fonts.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="$root/web/static/fonts"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# fetch <package> <version> <sha512 integrity> <licence file name> <file>...
fetch() {
	local pkg="$1" version="$2" integrity="$3" licence="$4"
	shift 4
	local url="https://registry.npmjs.org/@fontsource-variable/$pkg/-/$pkg-$version.tgz"
	echo "Downloading $url"
	curl -fsSL --retry 3 -o "$tmp/$pkg.tgz" "$url"
	local got
	got="sha512-$(openssl dgst -sha512 -binary "$tmp/$pkg.tgz" | base64 -w0)"
	if [[ "$got" != "$integrity" ]]; then
		echo "integrity mismatch for $url: got $got, want $integrity" >&2
		exit 1
	fi
	mkdir -p "$tmp/$pkg"
	tar -xzf "$tmp/$pkg.tgz" -C "$tmp/$pkg"
	for f in "$@"; do
		cp "$tmp/$pkg/package/files/$f" "$dest/$f"
	done
	cp "$tmp/$pkg/package/LICENSE" "$dest/$licence"
}

mkdir -p "$dest"
fetch saira 5.3.0 \
	"sha512-C1rm7n1OuEx4Se/SAB/5EeUuCviGFFnYTr5vxuQQm2YIOfgbQWx/P6qqhdXlVCBRm/1UzNWCtDK5jNbVmO6W2w==" \
	OFL-Saira.txt \
	saira-latin-standard-normal.woff2 saira-latin-ext-standard-normal.woff2
fetch atkinson-hyperlegible-next 5.3.0 \
	"sha512-UnLzNUy1RWoLDfydAatP4NaRm99X/WbQEph7q/PymJnCFIOOwKOx3pBmp5ys8trO0dM+CKGuJgeZP9fAR4mQpA==" \
	OFL-AtkinsonHyperlegibleNext.txt \
	atkinson-hyperlegible-next-latin-wght-normal.woff2 atkinson-hyperlegible-next-latin-ext-wght-normal.woff2 \
	atkinson-hyperlegible-next-latin-wght-italic.woff2
# No page sets Mono in italic, so its italic file is not shipped.
fetch atkinson-hyperlegible-mono 5.3.0 \
	"sha512-0IbthhO18EEpLe69h+cmrsxCkl4yk80gMk+fAN3+9M5zQvoR1JlkzoJRR6U9/UXLYpVBGEXu+Md2LO8TIp30dw==" \
	OFL-AtkinsonHyperlegibleMono.txt \
	atkinson-hyperlegible-mono-latin-wght-normal.woff2 atkinson-hyperlegible-mono-latin-ext-wght-normal.woff2
echo "The fonts are in $dest"
