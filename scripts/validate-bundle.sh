#!/usr/bin/env bash
set -euo pipefail

bundle_dir="${1:-}"
if [[ -z "$bundle_dir" || "$bundle_dir" == "-h" || "$bundle_dir" == "--help" ]]; then
  echo "Usage: validate-bundle.sh BUNDLE_DIR" >&2
  exit 2
fi

required=(
  "storctl-compose"
  "storctl-profiles.json"
  "drivers"
  "checksums.txt"
)

for item in "${required[@]}"; do
  if [[ ! -e "$bundle_dir/$item" ]]; then
    echo "FAIL missing $item"
    exit 1
  fi
done

(
  cd "$bundle_dir"
  shasum -a 256 -c checksums.txt >/dev/null
)

echo "OK bundle $bundle_dir"
