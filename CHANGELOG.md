# Changelog

## Unreleased

### Added

- Go single-binary `storctl-compose` CLI with built-in SSH/SFTP orchestration.
- Automatic 1823 NIC discovery and per-candidate apply attempts.
- Password and key-file host authentication in `hosts.yaml`.
- Release workflow that embeds `storctl-linux-arm64` into `storctl-compose`.
- Initial Ansible-based companion repository for `storctl`.
- Example inventory, profile, and driver matrix files.
- Offline bundle, validation, and report collection scripts.
- Detailed Chinese batch onboarding tutorial under `docs/tutorial.md`.
- High-fidelity local simulation suite under `tests/sim/`, covering OS/SP,
  artifact, driver, NetworkManager, QoS, mount, fallback, and check flows.

### Changed

- Ansible wrapper and playbooks moved to `legacy/ansible`; the Go binary is now the primary workflow.
- Offline bundle helper now packages `storctl-compose` instead of a standalone target-side `storctl` binary.
