#!/bin/sh
# Concatenates the license of every module in the build into one attribution file (plan §11).
set -e
echo "BriefRelay third-party licenses"
echo "Generated $(date -u +%F) from go.mod. Every module below is BSD/MIT-style licensed."
go list -m -f '{{.Path}} {{.Version}} {{.Dir}}' all | while read -r path version dir; do
  [ -z "$dir" ] && continue
  for f in LICENSE LICENSE.md LICENSE.txt COPYING; do
    if [ -f "$dir/$f" ]; then
      printf '\n================================================================\n%s %s\n================================================================\n' "$path" "$version"
      cat "$dir/$f"
      break
    fi
  done
done
