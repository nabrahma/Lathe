#!/usr/bin/env bash
# Packages a built application into a distributable archive.
#
# Deliberately plain: a zip on Windows, a compressed dmg-less zip on macOS, and
# a tarball on Linux. Platform installers (NSIS, dmg, AppImage) come later;
# an archive that works everywhere is better than an installer that works on
# one platform and blocks a release on the other two.
set -euo pipefail

name="${1:?artifact name required}"
version="${2:-dev}"

mkdir -p dist
shopt -s nullglob

case "$(uname -s)" in
  Darwin)
    # The .app bundle is a directory, so it has to be archived rather than
    # copied as a file.
    app=(build/bin/*.app)
    if [ ${#app[@]} -gt 0 ]; then
      ditto -c -k --sequesterRsrc --keepParent "${app[0]}" "dist/${name}-${version}.zip"
    fi
    ;;
  Linux)
    tar -czf "dist/${name}-${version}.tar.gz" -C build/bin .
    ;;
  *)
    # Windows runners have PowerShell, which is the only reliable zip here.
    powershell -NoProfile -Command \
      "Compress-Archive -Path 'build/bin/*' -DestinationPath 'dist/${name}-${version}.zip' -Force"
    ;;
esac

ls -la dist/
