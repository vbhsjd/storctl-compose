# Changelog

## Unreleased

### Added

- Go single-binary `storctl-compose` CLI with built-in SSH/SFTP orchestration.
- Automatic 1823 NIC discovery and per-candidate apply attempts.
- Password and key-file host authentication in `hosts.yaml`.
- Release workflow that embeds `storctl-linux-arm64` into `storctl-compose`.
- Fast-start release layout with root-level templates and standalone `storctl-linux-arm64`.
- Initial Ansible-based companion repository for `storctl`.
- Example inventory, profile, and driver matrix files.
- Offline bundle, validation, and report collection scripts.
- Detailed Chinese batch onboarding tutorial under `docs/tutorial.md`.
- High-fidelity local simulation suite under `tests/sim/`, covering OS/SP,
  artifact, driver, NetworkManager, QoS, mount, fallback, and check flows.
- 1823 Hilink readiness probing during `apply`, including optical/module/link
  diagnostics and per-NIC probe logs.
- Pre-apply `storctl check --json` guard so already-mounted hosts are skipped
  instead of being reconfigured.
- Per-host `--timeout` for copy, install-driver, apply, and check. SFTP uploads
  now close the SSH/SFTP connection when the timeout is hit.
- `storctl-compose report --csv result.csv` for exporting every host result,
  including successful hosts.
- `storctl-compose report --xlsx result.xlsx` for Excel-friendly reports with
  a filter row, wider columns, and `protocol` values of `rdma` or `tcp`.
- `hosts.csv` as the default host input format: `ip,password,user`, with
  `user` defaulting to `root`.
- `storctl-compose report --all` for viewing historical records outside the
  current hosts file.

### Changed

- Ansible wrapper and playbooks moved to `legacy/ansible`; the Go binary is now the primary workflow.
- Offline bundle helper now packages `storctl-compose` instead of a standalone target-side `storctl` binary.
- Public `compose.yaml` no longer exposes `nic_type`; `storctl-compose` is fixed to 1823 orchestration.
- Reports now include candidate NIC probe summaries and aggregate link/optical
  failure counters.
- Hilink probing now falls back from `hinicX` device names to Linux NIC names
  such as `eth3`, matching field behavior on some 1823 hosts.
- Release bundle now embeds `storctl` with stale VLAN parent repair and VLAN
  MTU rebuild handling.
- Release bundle now embeds `storctl` with NFSv3 TCP fallback defaults for lab
  storage servers that reject NFSv4.1 TCP.
- `storctl-compose report` now defaults to a compact human summary; detailed
  counters moved behind `--verbose`, and full machine output is available with
  `--json`.
- Compact report output now includes a success list as well as failures.
- Report CSV output is now intentionally small: `ip,command,status,code,message,protocol`.
- Non-root SSH users are allowed when the target has passwordless sudo.
- Default report output now filters by current `hosts.csv`, shows `MISS/not_run`
  for hosts without results, and reports ignored stale records.
- Batch command output now uses short per-host lines plus grouped failure
  summaries.
- Common SSH, NetworkManager, and NFS mount failures are normalized to stable
  codes such as `auth_failed`, `ssh_timeout`, `networkmanager_down`, and
  `mount_failed`.
