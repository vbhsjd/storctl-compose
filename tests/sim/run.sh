#!/usr/bin/env bash
set -u -o pipefail

sim_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
compose_root="$(cd "${sim_dir}/../.." && pwd)"
if [[ -n "${STORCTL_SOURCE_DIR:-}" ]]; then
  storctl_root="$(cd "${STORCTL_SOURCE_DIR}" && pwd)"
elif [[ -f "${compose_root}/../go.mod" ]]; then
  storctl_root="$(cd "${compose_root}/.." && pwd)"
elif [[ -f "${compose_root}/../storctl/go.mod" ]]; then
  storctl_root="$(cd "${compose_root}/../storctl" && pwd)"
else
  echo "FAIL can not find storctl source; set STORCTL_SOURCE_DIR" >&2
  exit 1
fi
work="${sim_dir}/.work"
bin="${work}/bin/storctl"
fakebin="${sim_dir}/fakebin"

pass=0
fail=0

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$1" | awk '{print $1}'
}

say() {
  printf '%s\n' "$*"
}

record_pass() {
  pass=$((pass + 1))
  say "PASS $*"
}

record_fail() {
  fail=$((fail + 1))
  say "FAIL $*"
}

run_step() {
  local node="$1" name="$2" expect="$3"
  shift 3
  local root="${work}/nodes/${node}"
  local report="${work}/reports/${node}"
  mkdir -p "${report}"
  (
    export STORCTL_SIM_ROOT="${root}"
    export STORCTL_SIM_ARCH="aarch64"
    export PATH="${fakebin}:$PATH"
    "$@"
  ) > "${report}/${name}.out" 2> "${report}/${name}.err"
  local rc=$?
  if [[ "${expect}" == "ok" && ${rc} -eq 0 ]]; then
    record_pass "${node}:${name}"
    return 0
  fi
  if [[ "${expect}" == "fail" && ${rc} -ne 0 ]]; then
    record_pass "${node}:${name} expected failure"
    return 0
  fi
  record_fail "${node}:${name} rc=${rc} expected=${expect}"
  tail -n 20 "${report}/${name}.out" "${report}/${name}.err" 2>/dev/null
  [[ -f "${root}/var/log/storctl-sim/commands.log" ]] && tail -n 20 "${root}/var/log/storctl-sim/commands.log"
  return 1
}

assert_file_contains() {
  local node="$1" file="$2" pattern="$3" label="$4"
  local root="${work}/nodes/${node}"
  if grep -Eq "${pattern}" "${root}/${file}" 2>/dev/null; then
    record_pass "${node}:${label}"
  else
    record_fail "${node}:${label} missing ${pattern} in ${file}"
  fi
}

assert_report_contains() {
  local node="$1" step="$2" pattern="$3" label="$4"
  local out="${work}/reports/${node}/${step}.out"
  local err="${work}/reports/${node}/${step}.err"
  if cat "${out}" "${err}" 2>/dev/null | grep -Eq "${pattern}"; then
    record_pass "${node}:${label}"
  else
    record_fail "${node}:${label} missing ${pattern}"
    tail -n 30 "${out}" "${err}" 2>/dev/null
  fi
}

write_os_release() {
  local root="$1" version_id="$2" version_text="$3" pretty="$4"
  mkdir -p "${root}/etc"
  cat > "${root}/etc/os-release" <<EOF
ID=openEuler
VERSION_ID="${version_id}"
VERSION="${version_text}"
PRETTY_NAME="${pretty}"
EOF
}

add_iface() {
  local root="$1" name="$2" state="$3" addrs="${4:-}"
  mkdir -p "${root}/sys/class/net/${name}"
  printf '%s\n' "${state}" > "${root}/sys/class/net/${name}/operstate"
  [[ -n "${addrs}" ]] && printf '%s\n' "${addrs}" > "${root}/sys/class/net/${name}/ipv4_addrs"
}

write_profile() {
  local root="$1"
  mkdir -p "${root}/etc/storctl"
  cat > "${root}/etc/storctl/profiles.json" <<'EOF'
{
  "profiles": {
    "c4": {
      "vlan_id": 172,
      "gateway": "172.27.0.1",
      "prefix": 18,
      "route_table": 5000,
      "mtu": 5500,
      "artifact_dir": "/root/storage_pkgs",
      "third_octet_map": {
        "17": 4,
        "21": 3
      },
      "mounts": [
        {"server": "172.27.1.1", "export": "/Share", "mount_point": "/mnt/share"},
        {"server": "172.27.1.1", "export": "/Weight", "mount_point": "/mnt/weight"}
      ]
    }
  }
}
EOF
}

write_artifacts_1823_sp4() {
  local root="$1"
  mkdir -p "${root}/root/storage_pkgs"
  printf 'broad 1823 package\n' > "${root}/root/storage_pkgs/SDK_LINUX-17.12.5.0-openEuler22.03-aarch64.tar.gz"
  printf 'sp4 1823 package\n' > "${root}/root/storage_pkgs/SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz"
  local broad sp4
  broad="$(sha256 "${root}/root/storage_pkgs/SDK_LINUX-17.12.5.0-openEuler22.03-aarch64.tar.gz")"
  sp4="$(sha256 "${root}/root/storage_pkgs/SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz")"
  cat > "${root}/root/storage_pkgs/storctl-artifacts.json" <<EOF
{
  "artifacts": [
    {
      "os_id": "openEuler",
      "os_version_prefix": "22.03",
      "arch": "aarch64",
      "nic_type": "1823",
      "file": "SDK_LINUX-17.12.5.0-openEuler22.03-aarch64.tar.gz",
      "sha256": "${broad}",
      "requires_repo": false
    },
    {
      "os_id": "openEuler",
      "os_version_prefix": "22.03-LTS-SP4",
      "arch": "aarch64",
      "nic_type": "1823",
      "file": "SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz",
      "sha256": "${sp4}",
      "requires_repo": false
    }
  ]
}
EOF
}

write_artifacts_cx7_sp2() {
  local root="$1"
  mkdir -p "${root}/root/storage_pkgs"
  printf 'cx7 package\n' > "${root}/root/storage_pkgs/MLNX_OFED_LINUX-24.03SP2-aarch64.tgz"
  local cx7
  cx7="$(sha256 "${root}/root/storage_pkgs/MLNX_OFED_LINUX-24.03SP2-aarch64.tgz")"
  cat > "${root}/root/storage_pkgs/storctl-artifacts.json" <<EOF
{
  "artifacts": [
    {
      "os_id": "openEuler",
      "os_version_prefix": "24.03-LTS-SP2",
      "arch": "aarch64",
      "nic_type": "cx7",
      "file": "MLNX_OFED_LINUX-24.03SP2-aarch64.tgz",
      "sha256": "${cx7}",
      "requires_repo": false
    }
  ]
}
EOF
}

write_artifacts_doca_sp2() {
  local root="$1"
  mkdir -p "${root}/root/storage_pkgs"
  printf 'doca host\n' > "${root}/root/storage_pkgs/doca-host-test.rpm"
  local doca
  doca="$(sha256 "${root}/root/storage_pkgs/doca-host-test.rpm")"
  cat > "${root}/root/storage_pkgs/storctl-artifacts.json" <<EOF
{
  "artifacts": [
    {
      "os_id": "openEuler",
      "os_version_prefix": "24.03-LTS-SP2",
      "arch": "aarch64",
      "nic_type": "cx7",
      "file": "doca-host-test.rpm",
      "sha256": "${doca}",
      "requires_repo": true
    }
  ]
}
EOF
}

setup_1823_node() {
  local name="$1" rdma="$2" systemd="$3"
  local root="${work}/nodes/${name}"
  mkdir -p "${root}/sim" "${root}/var/log/storctl-sim" "${root}/var/lib/storctl-sim"
  write_os_release "${root}" "22.03" "22.03 (LTS-SP4)" "openEuler 22.03 (LTS-SP4)"
  [[ "${systemd}" == "yes" ]] && mkdir -p "${root}/run/systemd/system"
  add_iface "${root}" "enp23s0f1" "up"
  add_iface "${root}" "ethmgmt0" "up" "80.5.17.113/22"
  write_profile "${root}"
  write_artifacts_1823_sp4 "${root}"
  printf 'Card num:1\nhinic0(CAL_2X200G_INTERNET)\n' > "${root}/sim/hinicadm3_info"
  [[ "${rdma}" == "ready" ]] && printf 'link mlx5_0/1 state ACTIVE physical_state LINK_UP netdev enp23s0f1\n' > "${root}/sim/rdma_link" || : > "${root}/sim/rdma_link"
}

setup_cx7_node() {
  local name="$1"
  local root="${work}/nodes/${name}"
  mkdir -p "${root}/sim" "${root}/run/systemd/system"
  write_os_release "${root}" "24.03" "24.03 (LTS-SP2)" "openEuler 24.03 (LTS-SP2)"
  add_iface "${root}" "enp194s0f1np1" "up"
  add_iface "${root}" "ethmgmt0" "up" "80.5.21.122/22"
  write_profile "${root}"
  write_artifacts_cx7_sp2 "${root}"
  printf 'mlx5_1 port 1 ==> enp194s0f1np1 (Up)\n' > "${root}/sim/ibdev2netdev"
  printf 'link mlx5_1/1 state ACTIVE physical_state LINK_UP netdev enp194s0f1np1\n' > "${root}/sim/rdma_link"
}

setup_doca_node() {
  local name="$1"
  local root="${work}/nodes/${name}"
  setup_cx7_node "${name}"
  write_artifacts_doca_sp2 "${root}"
}

setup_management_guard_node() {
  local name="$1"
  local root="${work}/nodes/${name}"
  setup_1823_node "${name}" "ready" "yes"
  printf '80.5.17.113/22\n' > "${root}/sys/class/net/enp23s0f1/ipv4_addrs"
}

setup_ambiguous_node() {
  local name="$1"
  local root="${work}/nodes/${name}"
  setup_1823_node "${name}" "ready" "yes"
  printf 'duplicate\n' > "${root}/root/storage_pkgs/SDK_LINUX-duplicate-openEuler22.03SP4-aarch64.tar.gz"
  local dup
  dup="$(sha256 "${root}/root/storage_pkgs/SDK_LINUX-duplicate-openEuler22.03SP4-aarch64.tar.gz")"
  python3 - "$root" "$dup" <<'PY'
import json, sys
root, dup = sys.argv[1], sys.argv[2]
path = f"{root}/root/storage_pkgs/storctl-artifacts.json"
data = json.load(open(path))
data["artifacts"].append({
    "os_id": "openEuler",
    "os_version_prefix": "22.03-LTS-SP4",
    "arch": "aarch64",
    "nic_type": "1823",
    "file": "SDK_LINUX-duplicate-openEuler22.03SP4-aarch64.tar.gz",
    "sha256": dup,
    "requires_repo": False,
})
json.dump(data, open(path, "w"), indent=2)
PY
}

setup_sha_mismatch_node() {
  local name="$1"
  setup_1823_node "${name}" "ready" "yes"
  python3 - "${work}/nodes/${name}/root/storage_pkgs/storctl-artifacts.json" <<'PY'
import json, sys
path = sys.argv[1]
data = json.load(open(path))
data["artifacts"][1]["sha256"] = "0000"
json.dump(data, open(path, "w"), indent=2)
PY
}

prepare() {
  rm -rf "${work}"
  mkdir -p "${work}/bin" "${work}/reports" "${work}/nodes"
  (cd "${storctl_root}" && go build -o "${bin}" ./cmd/storctl)
  setup_1823_node "oe22sp4-1823-rdma" "ready" "yes"
  setup_1823_node "oe22sp4-1823-tcp-fallback" "empty" "yes"
  setup_1823_node "oe22sp4-1823-no-fallback" "empty" "yes"
  setup_1823_node "oe22sp4-1823-no-systemd" "ready" "no"
  setup_1823_node "oe22sp4-1823-existing-tcp" "ready" "yes"
  mkdir -p "${work}/nodes/oe22sp4-1823-existing-tcp/var/lib/storctl-sim"
  printf '/mnt/share|nfs4|vers=3,proto=tcp,nconnect=8|172.27.1.1:/Share\n' > "${work}/nodes/oe22sp4-1823-existing-tcp/var/lib/storctl-sim/mounts.tsv"
  setup_cx7_node "oe24sp2-cx7"
  setup_doca_node "oe24sp2-cx7-doca"
  setup_management_guard_node "management-nic-guard"
  setup_ambiguous_node "ambiguous-artifact"
  setup_sha_mismatch_node "sha-mismatch"
}

run_suite() {
  run_step "oe22sp4-1823-rdma" "facts" ok "${bin}" facts --json
  assert_report_contains "oe22sp4-1823-rdma" "facts" '"normalized_version": "22.03-lts-sp4"' "facts normalized sp4"
  run_step "oe22sp4-1823-rdma" "validate-artifacts" ok "${bin}" validate-artifacts --artifact-dir /root/storage_pkgs
  run_step "oe22sp4-1823-rdma" "install-driver" ok "${bin}" install-driver --nic-type 1823 --artifact-dir /root/storage_pkgs
  assert_report_contains "oe22sp4-1823-rdma" "install-driver" 'OK artifact SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz' "sp4 artifact selected"
  assert_file_contains "oe22sp4-1823-rdma" "var/log/storctl-sim/commands.log" 'storctl-sim-sh .*install.sh.*roce' "1823 installer called"
  run_step "oe22sp4-1823-rdma" "plan" ok "${bin}" plan --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113
  assert_report_contains "oe22sp4-1823-rdma" "plan" 'OK data-ip 172.27.4.113/18' "profile derived data ip"
  run_step "oe22sp4-1823-rdma" "apply" ok "${bin}" apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823 --qos apply
  assert_file_contains "oe22sp4-1823-rdma" "var/log/storctl-sim/commands.log" 'nmcli con mod data0.172' "nmcli vlan modified"
  assert_file_contains "oe22sp4-1823-rdma" "var/log/storctl-sim/commands.log" 'mount .*proto=rdma' "rdma mount called"
  assert_file_contains "oe22sp4-1823-rdma" "etc/systemd/system/mnt-share.automount" 'Where=/mnt/share' "systemd automount written"
  run_step "oe22sp4-1823-rdma" "check-json" ok "${bin}" check --json
  assert_report_contains "oe22sp4-1823-rdma" "check-json" '"code": "mount_rdma"' "check reports rdma mount"

  run_step "oe22sp4-1823-tcp-fallback" "apply" ok "${bin}" apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823 --allow-tcp-fallback
  assert_file_contains "oe22sp4-1823-tcp-fallback" "var/lib/storctl/state.json" '"degraded": true' "tcp fallback state degraded"
  assert_file_contains "oe22sp4-1823-tcp-fallback" "var/log/storctl-sim/commands.log" 'mount .*proto=tcp' "tcp fallback mount called"
  run_step "oe22sp4-1823-tcp-fallback" "check-json" ok "${bin}" check --json
  assert_report_contains "oe22sp4-1823-tcp-fallback" "check-json" '"code": "tcp_fallback_degraded"' "check reports degraded"

  run_step "oe22sp4-1823-no-fallback" "apply" fail "${bin}" apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823
  assert_report_contains "oe22sp4-1823-no-fallback" "apply" 'FAIL driver 1823' "no fallback fails driver"

  run_step "oe22sp4-1823-existing-tcp" "apply" ok "${bin}" apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823
  assert_file_contains "oe22sp4-1823-existing-tcp" "var/log/storctl-sim/commands.log" '^umount /mnt/share' "existing tcp unmounted"
  assert_file_contains "oe22sp4-1823-existing-tcp" "var/log/storctl-sim/commands.log" 'mount .*proto=rdma' "existing tcp remounted rdma"

  run_step "oe22sp4-1823-no-systemd" "apply" ok "${bin}" apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823
  assert_file_contains "oe22sp4-1823-no-systemd" "etc/fstab" '172.27.1.1:/Share /mnt/share nfs .*proto=rdma' "fstab fallback written"

  run_step "oe24sp2-cx7" "install-driver" ok "${bin}" install-driver --nic-type cx7 --artifact-dir /root/storage_pkgs
  assert_report_contains "oe24sp2-cx7" "install-driver" 'OK artifact MLNX_OFED_LINUX-24.03SP2-aarch64.tgz' "cx7 sp2 artifact selected"
  assert_file_contains "oe24sp2-cx7" "var/log/storctl-sim/commands.log" 'mlnxofedinstall' "cx7 installer called"
  run_step "oe24sp2-cx7" "apply" ok "${bin}" apply --profile c4 --nic enp194s0f1np1 --mgmt-ip 80.5.21.122 --nic-type cx7 --qos apply
  assert_file_contains "oe24sp2-cx7" "var/log/storctl-sim/commands.log" 'mlnx_qos' "cx7 qos called"

  run_step "oe24sp2-cx7-doca" "install-driver" fail "${bin}" install-driver --nic-type cx7 --artifact-dir /root/storage_pkgs
  assert_report_contains "oe24sp2-cx7-doca" "install-driver" 'requires a configured dnf repo' "doca requires repo guarded"

  run_step "management-nic-guard" "apply" fail "${bin}" apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823
  assert_report_contains "management-nic-guard" "apply" 'this looks like the SSH management interface' "management nic rejected"
  if grep -q 'nmcli con mod' "${work}/nodes/management-nic-guard/var/log/storctl-sim/commands.log" 2>/dev/null; then
    record_fail "management-nic-guard:nmcli should not mutate after guard"
  else
    record_pass "management-nic-guard:no nmcli mutation"
  fi

  run_step "ambiguous-artifact" "install-driver" fail "${bin}" install-driver --nic-type 1823 --artifact-dir /root/storage_pkgs
  assert_report_contains "ambiguous-artifact" "install-driver" 'ambiguous artifacts' "ambiguous artifact detected"
  run_step "sha-mismatch" "install-driver" fail "${bin}" install-driver --nic-type 1823 --artifact-dir /root/storage_pkgs
  assert_report_contains "sha-mismatch" "install-driver" 'sha256 mismatch' "sha mismatch detected"
}

prepare
run_suite

say "----"
say "reports: ${work}/reports"
say "nodes:   ${work}/nodes"
say "PASS=${pass} FAIL=${fail}"
[[ ${fail} -eq 0 ]]
