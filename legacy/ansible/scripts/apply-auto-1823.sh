#!/usr/bin/env bash
set -euo pipefail

storctl_bin="${STORCTL_BIN:-/usr/local/bin/storctl}"
profile_file="${STORCTL_PROFILE_FILE:-/etc/storctl/profiles.json}"
profile="${STORCTL_PROFILE:?STORCTL_PROFILE is required}"
mgmt_ip="${STORCTL_MGMT_IP:?STORCTL_MGMT_IP is required}"
qos="${STORCTL_QOS:-off}"
allow_tcp="${STORCTL_ALLOW_TCP_FALLBACK:-1}"
log_dir="${STORCTL_LOG_DIR:-/var/lib/storctl-compose/apply}"
sim_root="${STORCTL_SIM_ROOT:-}"
sys_net="/sys/class/net"
if [[ -n "${sim_root}" ]]; then
  sys_net="${sim_root}/sys/class/net"
  if [[ "${log_dir}" == /* ]]; then
    log_dir="${sim_root}${log_dir}"
  fi
fi

mkdir -p "${log_dir}"

is_ignored_iface() {
  case "$1" in
    lo|docker*|veth*|virbr*|br*|bond*|team*|tun*|tap*|kube*|cni*|flannel*|cali*|data0.*|*.+[0-9]*)
      return 0
      ;;
  esac
  [[ "$1" == *"."* ]]
}

iface_has_mgmt_ip() {
  local nic="$1"
  ip -o -4 addr show dev "${nic}" 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | grep -Fxq "${mgmt_ip}"
}

iface_driver() {
  ethtool -i "$1" 2>/dev/null | awk -F': *' '$1 == "driver" {print $2; exit}'
}

iface_speed() {
  local nic="$1"
  if [[ -r "${sys_net}/${nic}/speed" ]]; then
    cat "${sys_net}/${nic}/speed" 2>/dev/null || true
    return
  fi
  ethtool "${nic}" 2>/dev/null | awk -F': *' '$1 ~ /Speed/ {gsub(/Mb\/s/, "", $2); print $2; exit}'
}

iface_up() {
  [[ "$(cat "${sys_net}/$1/operstate" 2>/dev/null || true)" == "up" ]]
}

iface_carrier() {
  [[ "$(cat "${sys_net}/$1/carrier" 2>/dev/null || true)" == "1" ]]
}

iface_has_ipv4() {
  ip -o -4 addr show dev "$1" 2>/dev/null | grep -q ' inet '
}

discover_candidates() {
  local nic driver speed score carrier fast noip up
  for path in "${sys_net}"/*; do
    [[ -e "${path}" ]] || continue
    nic="$(basename "${path}")"
    if is_ignored_iface "${nic}"; then
      echo "SKIP nic ${nic} ignored" >&2
      continue
    fi
    if [[ ! -e "${path}/device" ]]; then
      echo "SKIP nic ${nic} no device" >&2
      continue
    fi
    if iface_has_mgmt_ip "${nic}"; then
      echo "SKIP nic ${nic} management-ip" >&2
      continue
    fi
    driver="$(iface_driver "${nic}")"
    if [[ "${driver}" != hinic3* && "${driver}" != hinic* ]]; then
      echo "SKIP nic ${nic} driver=${driver:-unknown}" >&2
      continue
    fi
    speed="$(iface_speed "${nic}")"
    [[ "${speed}" =~ ^[0-9]+$ ]] || speed=0
    carrier=0; iface_carrier "${nic}" && carrier=1
    fast=0; [[ "${speed}" -ge 100000 ]] && fast=1
    noip=1; iface_has_ipv4 "${nic}" && noip=0
    up=0; iface_up "${nic}" && up=1
    score=$((carrier * 1000 + fast * 100 + noip * 10 + up))
    printf '%04d %s\n' "${score}" "${nic}"
  done | sort -rn | awk '{print $2}'
}

candidates=()
while IFS= read -r candidate; do
  [[ -n "${candidate}" ]] && candidates+=("${candidate}")
done < <(discover_candidates)
if [[ "${#candidates[@]}" -eq 0 ]]; then
  echo "FAIL no_candidate_nic"
  echo "reason: no physical 1823 NIC found by ethtool -i"
  echo "next: check cabling, driver, and run: ip -br link; ethtool -i <nic>"
  exit 1
fi

echo "OK candidates ${candidates[*]}"

last_rc=1
for nic in "${candidates[@]}"; do
  out="${log_dir}/${nic}.out"
  err="${log_dir}/${nic}.err"
  cmd=(
    "${storctl_bin}" apply
    --profile-file "${profile_file}"
    --profile "${profile}"
    --nic "${nic}"
    --nic-type 1823
    --mgmt-ip "${mgmt_ip}"
    --qos "${qos}"
  )
  if [[ "${allow_tcp}" == "1" || "${allow_tcp}" == "true" ]]; then
    cmd+=(--allow-tcp-fallback)
  fi
  echo "TRY nic ${nic}"
  if "${cmd[@]}" >"${out}" 2>"${err}"; then
    cat "${out}"
    "${storctl_bin}" check --json > "${log_dir}/${nic}.check.json" 2>/dev/null || true
    echo "OK selected-nic ${nic}"
    exit 0
  else
    last_rc=$?
  fi
  echo "WARN nic ${nic} failed rc=${last_rc}"
  sed -n '1,80p' "${out}" || true
  sed -n '1,80p' "${err}" || true
done

echo "FAIL all_candidate_nics"
echo "reason: no candidate NIC completed storctl apply"
echo "next: inspect ${log_dir}/*.out and ${log_dir}/*.err"
exit "${last_rc}"
