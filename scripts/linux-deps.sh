#!/usr/bin/env bash
# Installs the Go toolchain and WebKitGTK development headers across the three
# distro families Lathe builds on. Package names differ per distro; the
# webkit2gtk 4.0 vs 4.1 split is the reason this script exists.
set -euo pipefail

if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends \
    ca-certificates curl git build-essential pkg-config \
    libgtk-3-dev libwebkit2gtk-4.1-dev golang-go
elif command -v dnf >/dev/null 2>&1; then
  dnf -y install \
    ca-certificates curl git gcc pkgconf-pkg-config \
    gtk3-devel webkit2gtk4.1-devel golang
elif command -v pacman >/dev/null 2>&1; then
  pacman -Sy --noconfirm \
    ca-certificates curl git base-devel pkgconf \
    gtk3 webkit2gtk-4.1 go
else
  echo "unsupported distro: no apt-get, dnf or pacman" >&2
  exit 1
fi

go version
pkg-config --modversion webkit2gtk-4.1 || true
