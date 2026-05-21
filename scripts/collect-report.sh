#!/usr/bin/env bash
set -euo pipefail

report_dir="${1:-reports}"
if [[ ! -d "$report_dir" ]]; then
  echo "FAIL report directory not found: $report_dir" >&2
  exit 1
fi

if command -v jq >/dev/null 2>&1; then
  jq -r '
    . as $doc
    | [
        input_filename,
        ([.checks[]? | select(.status == "FAIL")] | length),
        ([.checks[]? | select(.code == "tcp_fallback_degraded")] | length),
        ([.checks[]? | select(.code == "rdma_link_empty")] | length)
      ]
    | @tsv
  ' "$report_dir"/*.json | awk 'BEGIN {print "file\tfailures\tdegraded_tcp\trdma_link_empty"} {print}'
else
  echo "WARN jq not found; listing report files only"
  find "$report_dir" -name '*.json' -print | sort
fi
