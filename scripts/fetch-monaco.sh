#!/usr/bin/env bash
# Downloads the Monaco editor from the npm registry and copies its AMD build
# (package/min/vs) to web/static/vendor/monaco/vs, where the IDE page loads
# it. The tarball is checked against the sha512 integrity that the registry
# publishes for that version, pinned here: Renovate moves the version and
# the integrity together. Run it once after a checkout; the web image runs it
# at build time.
#
# Usage: scripts/fetch-monaco.sh
# To try another release: MONACO_VERSION=x.y.z MONACO_INTEGRITY=sha512-... scripts/fetch-monaco.sh
# (npm view monaco-editor@x.y.z dist.integrity prints it)
set -euo pipefail

version="${MONACO_VERSION:-0.57.0}"
if [[ "$version" == "0.57.0" ]]; then
	integrity="${MONACO_INTEGRITY:-sha512-5BkI9KGoqrNvBGUe15/QlZq3OooZ8WLg1AxTpaqHRCP3HNpzPPZKE2EDz8M7c+VRmCeUw1Brp4cx/PWm3kI/5A==}"
else
	integrity="${MONACO_INTEGRITY:?set MONACO_INTEGRITY to the sha512 integrity of monaco-editor@$version}"
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

# openssl base64 -A, not base64 -w0: the base64 of macOS has no -w.
got="sha512-$(openssl dgst -sha512 -binary "$tmp/monaco.tgz" | openssl base64 -A)"
if [[ "$got" != "$integrity" ]]; then
	echo "integrity mismatch for $url: got $got, want $integrity" >&2
	exit 1
fi

tar -xzf "$tmp/monaco.tgz" -C "$tmp" package/min/vs package/LICENSE package/ThirdPartyNotices.txt
rm -rf "$dest"
mkdir -p "$dest"
mv "$tmp/package/min/vs" "$dest/vs"
cp "$tmp/package/LICENSE" "$tmp/package/ThirdPartyNotices.txt" "$dest/"
echo "$version" >"$dest/VERSION"
echo "Monaco $version is in $dest"
