#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: build-bundle.sh --compose-bin PATH --profiles PATH --drivers DIR --out DIR --name NAME [--storctl PATH] [--config PATH] [--hosts PATH] [--matrix PATH]

Build an offline storctl-compose bundle without network access.
USAGE
}

compose_bin=""
storctl_bin=""
profiles=""
matrix=""
drivers=""
out_dir=""
name=""
config=""
hosts=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --compose-bin) compose_bin="$2"; shift 2 ;;
    --storctl) storctl_bin="$2"; shift 2 ;;
    --profiles) profiles="$2"; shift 2 ;;
    --matrix) matrix="$2"; shift 2 ;;
    --drivers) drivers="$2"; shift 2 ;;
    --out) out_dir="$2"; shift 2 ;;
    --name) name="$2"; shift 2 ;;
    --config) config="$2"; shift 2 ;;
    --hosts) hosts="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

for value in "$compose_bin" "$profiles" "$drivers" "$out_dir" "$name"; do
  if [[ -z "$value" ]]; then
    usage >&2
    exit 2
  fi
done

for path in "$compose_bin" "$profiles" "$drivers"; do
  if [[ ! -e "$path" ]]; then
    echo "missing: $path" >&2
    exit 1
  fi
done
for path in "$storctl_bin" "$matrix" "$config" "$hosts"; do
  if [[ -n "$path" && ! -e "$path" ]]; then
    echo "missing: $path" >&2
    exit 1
  fi
done

mkdir -p "$out_dir"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

bundle_dir="$work_dir/$name"
mkdir -p "$bundle_dir/drivers"
cp "$compose_bin" "$bundle_dir/storctl-compose"
cp "$profiles" "$bundle_dir/storctl-profiles.json"
cp -R "$drivers"/. "$bundle_dir/drivers/"

[[ -n "$storctl_bin" ]] && cp "$storctl_bin" "$bundle_dir/storctl-linux-arm64"
[[ -n "$matrix" ]] && cp "$matrix" "$bundle_dir/driver-matrix.yaml"
[[ -n "$config" ]] && cp "$config" "$bundle_dir/compose.yaml"
[[ -n "$hosts" ]] && cp "$hosts" "$bundle_dir/hosts.yaml"

if [[ -f "$drivers/storctl-artifacts.json" ]]; then
  cp "$drivers/storctl-artifacts.json" "$bundle_dir/storctl-artifacts.json"
fi

(
  cd "$bundle_dir"
  find . -type f ! -name checksums.txt -print | sort | xargs shasum -a 256 > checksums.txt
)

tar_path="$out_dir/$name.tar.gz"
tar -C "$work_dir" -czf "$tar_path" "$name"
echo "OK bundle $tar_path"
