# storctl-compose

`storctl-compose` is the batch companion for `storctl`: a single Go binary that connects to hosts over SSH/SFTP, copies `storctl`, profiles, and offline driver artifacts, then auto-tries 1823 NICs until storage is mounted.

No Ansible, sshpass, Python, or hand-written storage NIC names are required.

## Fast Start

Prepare:

- `hosts.yaml`: host IP, root user, password or key.
- `compose.yaml`: profile, local driver directory, remote paths.
- `drivers/`: offline 1823 driver packages and `storctl-artifacts.json`.

```bash
cp examples/hosts.yaml hosts.yaml
cp examples/compose.yaml compose.yaml
cp examples/storctl-profiles.json storctl-profiles.json
mkdir -p drivers reports
```

Run:

```bash
storctl-compose copy --hosts hosts.yaml --config compose.yaml
storctl-compose install-driver --hosts hosts.yaml --config compose.yaml
storctl-compose apply --hosts hosts.yaml --config compose.yaml
storctl-compose check --hosts hosts.yaml --config compose.yaml
storctl-compose report --report-dir reports
```

## Commands

```bash
storctl-compose copy             # upload storctl, profile, drivers
storctl-compose install-driver   # install 1823 driver explicitly; no auto reboot
storctl-compose apply            # auto-select 1823 NIC and mount storage
storctl-compose check            # collect storctl check --json
storctl-compose report           # summarize reports/
storctl-compose version --json
```

Useful flags:

```bash
--concurrency 30
--limit node-a,node-b
--upgrade-firmware
```

## Notes

- Only 1823 orchestration is supported; use `storctl` directly for CX7.
- Targets must allow root SSH login.
- `apply` never installs drivers; run `install-driver` first.
- TCP fallback is enabled by default and reported as degraded.
- Real driver packages are not stored in the public repo or public releases.
- Release binaries embed `storctl-linux-arm64`; source builds may set `storctl_bin` in `compose.yaml`.

See [docs/tutorial.md](docs/tutorial.md) for details.
