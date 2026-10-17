#!/usr/bin/env bash
# Downloads the 2 font families of the site from the npm registry and
# copies the files the site uses to web/static/fonts/. Both are OFL 1.1.
# It also cuts the round parentheses out of Inter (OFL 1.1): B612 draws
# "(" and ")" almost like "[" and "]", and in a Go course "Validate()"
# must not read as "Validate[]". The cut needs pyftsubset (pip install
# fonttools brotli).
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
	local url="https://registry.npmjs.org/@fontsource/$pkg/-/$pkg-$version.tgz"
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
fetch b612 5.3.0 \
	"sha512-2AQPrSFDI9K00w/lPlgfcqve+ZPdRSCttvVUxcmv+UrnwwaKLMym+/uXmM7+dgPCVBIV9omoXUabFoyBFpc1Vg==" \
	OFL-B612.txt \
	b612-latin-400-normal.woff2 b612-latin-400-italic.woff2 b612-latin-700-normal.woff2
fetch spline-sans-mono 5.3.0 \
	"sha512-9MJxL52KuuVskwM3LbCwEP/L3ClsHsCd18gwmm5df/ZFh/TIg3LE9DT4pLyB5ptKCvEEi7JwclAe8HhCfzlBWg==" \
	OFL-SplineSansMono.txt \
	spline-sans-mono-latin-400-normal.woff2 spline-sans-mono-latin-ext-400-normal.woff2
fetch inter 5.2.8 \
	"sha512-P6r5WnJoKiNVV+zvW2xM13gNdFhAEpQ9dQJHt3naLvfg+LkF2ldgSLiF4T41lf1SQCM9QmkqPTn4TH568IRagg==" \
	OFL-Inter.txt
for w in 400 700; do
	pyftsubset "$tmp/inter/package/files/inter-latin-$w-normal.woff2" --unicodes=U+0028,U+0029 \
		--flavor=woff2 --layout-features='' --no-hinting --desubroutinize \
		--output-file="$dest/inter-parens-$w.woff2"
done
echo "The fonts are in $dest"
