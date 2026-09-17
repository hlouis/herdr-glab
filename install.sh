#!/bin/sh
# Build the plugin binary. herdr runs this once during `herdr plugin install`,
# after you confirm it. A local Go toolchain builds the checked-out source;
# without Go the matching release binary is downloaded instead.
set -eu

REPO="hlouis/herdr-glab"
BIN="bin/herdr-glab"

mkdir -p bin

if command -v go >/dev/null 2>&1; then
	go build -o "$BIN" ./cmd/herdr-glab
	echo "built $BIN with $(go version)"
	exit 0
fi

version="$(sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml | head -1)"
if [ -z "$version" ]; then
	echo "cannot read version from herdr-plugin.toml" >&2
	exit 1
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
x86_64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*) echo "no prebuilt binary for $(uname -m); install Go and try again" >&2; exit 1 ;;
esac

url="https://github.com/$REPO/releases/download/v$version/herdr-glab_v${version}_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Go not found; downloading $url"
curl -fsSL "$url" -o "$tmp/herdr-glab.tar.gz"
tar -xzf "$tmp/herdr-glab.tar.gz" -C "$tmp"
mv "$tmp/herdr-glab" "$BIN"
chmod +x "$BIN"
echo "installed $BIN from release v$version"
