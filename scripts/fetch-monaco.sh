#!/usr/bin/env bash
# Downloads the Monaco editor from the npm registry and copies its AMD build
# (package/min/vs) to web/static/vendor/monaco/vs, where the IDE page loads
# it. The tarball is checked against a pinned SHA-256. Run it once after a
# checkout; the web image runs it at build time.
#
# Usage: scripts/fetch-monaco.sh
# To try another release: MONACO_VERSION=x.y.z MONACO_SHA256=<sha256> scripts/fetch-monaco.sh
set -euo pipefail

version="${MONACO_VERSION:-0.57.0}"
if [[ "$version" == "0.57.0" ]]; then
	sha256="${MONACO_SHA256:-3ea1712fbacd3290cf4751007e3a5b57cc279767607812d4c3e57925fb0b05c2}"
else
	sha256="${MONACO_SHA256:?set MONACO_SHA256 to the SHA-256 of the monaco-editor-$version tarball}"
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dest="$root/web/static/vendor/monaco"

if [[ -f "$dest/VERSION" && "$(cat "$dest/VERSION")" == "$version" && -f "$dest/vs/loader.js" ]]; then
	echo "Monaco $version is already in $dest"
	exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

url="https://registry.npmjs.org/monaco-editor/-/monaco-editor-$version.tgz"
echo "Downloading $url"
curl -fsSL --retry 3 -o "$tmp/monaco.tgz" "$url"

if command -v sha256sum >/dev/null; then
	got="$(sha256sum "$tmp/monaco.tgz" | cut -d' ' -f1)"
else
	got="$(shasum -a 256 "$tmp/monaco.tgz" | cut -d' ' -f1)"
fi
if [[ "$got" != "$sha256" ]]; then
	echo "SHA-256 mismatch for $url: got $got, want $sha256" >&2
	exit 1
fi

tar -xzf "$tmp/monaco.tgz" -C "$tmp" package/min/vs package/LICENSE package/ThirdPartyNotices.txt
rm -rf "$dest"
mkdir -p "$dest"
mv "$tmp/package/min/vs" "$dest/vs"
cp "$tmp/package/LICENSE" "$tmp/package/ThirdPartyNotices.txt" "$dest/"
echo "$version" >"$dest/VERSION"
echo "Monaco $version is in $dest"
