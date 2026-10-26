#!/bin/sh
# Download the ArchUnit, slf4j, and JUnit console jars pinned in jars.lock.
set -eu
cd "$(dirname "$0")"
mkdir -p lib

check() {
  file="$1"
  sum="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    echo "${sum}  ${file}" | sha256sum -c -
  else
    echo "${sum}  ${file}" | shasum -a 256 -c -
  fi
}

while read -r name sum url; do
  case "$name" in
    "" | \#*) continue ;;
  esac
  dest="lib/${name}"
  if [ -f "$dest" ]; then
    check "$dest" "$sum"
    continue
  fi
  tmp="$(mktemp)"
  curl -fsSL -o "$tmp" "$url"
  check "$tmp" "$sum"
  mv "$tmp" "$dest"
done < jars.lock
