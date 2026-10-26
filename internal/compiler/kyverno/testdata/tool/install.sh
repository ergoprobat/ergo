#!/bin/sh
# Download the Kyverno CLI pinned in bin.lock.
set -eu
cd "$(dirname "$0")"
mkdir -p bin
if [ -x bin/kyverno ]; then
	exit 0
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*)
	echo "unsupported architecture ${arch}" >&2
	exit 1
	;;
esac
case "$os" in
linux | darwin) ;;
*)
	echo "unsupported operating system ${os}" >&2
	exit 1
	;;
esac

sum=""
url=""
while read -r name line_sum line_url; do
	case "$name" in
	"" | \#*) continue ;;
	esac
	case "$name" in
	kyverno-*-"$os"-"$arch")
		if [ -n "$url" ]; then
			echo "bin.lock has two Kyverno builds for ${os}/${arch}" >&2
			exit 1
		fi
		sum=$line_sum
		url=$line_url
		;;
	esac
done < bin.lock
if [ -z "$url" ]; then
	echo "bin.lock has no Kyverno build for ${os}/${arch}" >&2
	exit 1
fi

check() {
	file="$1"
	want="$2"
	if command -v sha256sum >/dev/null 2>&1; then
		echo "${want}  ${file}" | sha256sum -c -
	else
		echo "${want}  ${file}" | shasum -a 256 -c -
	fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
archive="${tmp}/kyverno.tar.gz"
curl -fsSL -o "$archive" "$url"
check "$archive" "$sum"
tar -xzf "$archive" -C "$tmp"
if [ ! -f "${tmp}/kyverno" ]; then
	echo "archive has no kyverno binary" >&2
	exit 1
fi
mv "${tmp}/kyverno" bin/kyverno
chmod 755 bin/kyverno
