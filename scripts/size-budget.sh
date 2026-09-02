#!/usr/bin/env bash
# Fails the build when a core installer exceeds its hard limit.
set -euo pipefail

case "$(uname -s)" in
  Darwin)  limit_mb=85 ;;
  Linux)   limit_mb=75 ;;
  MINGW*|MSYS*|CYGWIN*) limit_mb=70 ;;
  *)       limit_mb=85 ;;
esac

status=0
shopt -s nullglob
for f in build/bin/*; do
  [ -f "$f" ] || continue
  bytes=$(wc -c <"$f")
  mb=$(( bytes / 1000000 ))
  printf '%-40s %5s MB (limit %s MB)\n' "$(basename "$f")" "$mb" "$limit_mb"
  if [ "$mb" -gt "$limit_mb" ]; then
    echo "size budget exceeded: $(basename "$f") is ${mb} MB" >&2
    status=1
  fi
done
exit $status
