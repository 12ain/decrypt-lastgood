#!/usr/bin/env bash
# Packages the desktop client in gui/: an .app and a .dmg on macOS, an .exe and
# an installer on Windows, an executable and packages on Linux.
#
# Needs the toolchain of go.mod (1.27.3+, which MyGo requires). MyGo needs no
# cgo, so any machine builds the app of every platform; signing and disk images
# of macOS apps need a Mac.
#
#   ./build-gui.sh                                        # this machine
#   ./build-gui.sh -platform darwin/universal,windows/amd64,linux/amd64
#
# The builds land in gui/build/<os>-<arch>/; see docs/GUI.md.
set -euo pipefail

cd "$(dirname "$0")/gui"
exec go tool mygo build "$@"
