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
        ([.checks[]? | select(.code == "rdma_link_empty")] | length),
        ([.checks[]? | select(.code == "driver_not_ready")] | length),
        ([.checks[]? | select(.code == "no_candidate_nic")] | length)
      ]
    | @tsv
  ' "$report_dir"/*.json | awk 'BEGIN {print "file\tfailures\tdegraded_tcp\trdma_link_empty\tdriver_not_ready\tno_candidate_nic"} {print}'
else
  echo "WARN jq not found; listing report files only"
  find "$report_dir" -name '*.json' -print | sort
fi
