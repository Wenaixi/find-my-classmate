#!/usr/bin/env sh
set -eu
TARGET=artifacts/MODIFIED_FILE.txt
printf 'branch=baseline\nresult=unchanged\n' > "$TARGET"
printf 'rollback restored %s\n' "$TARGET"
