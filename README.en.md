# storctl-compose

`storctl-compose` is the beginner-friendly 1823 batch onboarding tool for `storctl`. It is shipped as a GitHub Release zip: unzip it, fill in the YAML/JSON templates, put offline driver packages under `drivers/`, then run the bundled binary.

No Ansible, sshpass, Python, or hand-written storage NIC names are required.

## Fast Start

```bash
unzip storctl-compose-*.zip
cd storctl-compose-*
cp hosts.yaml.example hosts.yaml
mkdir -p drivers reports
```

Edit:

- `hosts.yaml`: target host IP, root user, password or key.
- `compose.yaml`: profile name, local driver directory, remote paths.
- `storctl-profiles.json`: VLAN, gateway, IP derivation, and mounts.

Put offline 1823 driver packages and `storctl-artifacts.json` under `drivers/`, then run one host first:

```bash
./storctl-compose copy --limit node-57-122
./storctl-compose install-driver --limit node-57-122
./storctl-compose apply --limit node-57-122
./storctl-compose check --limit node-57-122
./storctl-compose report
```

Then run all hosts:

```bash
./storctl-compose copy
./storctl-compose install-driver
./storctl-compose apply
./storctl-compose check
./storctl-compose report
```

## Defaults

```text
--hosts hosts.yaml
--config compose.yaml
--report-dir reports
--concurrency 30
```

Useful flags:

```bash
./storctl-compose apply --limit node-a,node-b
./storctl-compose apply --concurrency 50
./storctl-compose install-driver --upgrade-firmware
./storctl-compose version --json
```

## Release Package

Release zips contain:

```text
storctl-compose
storctl-linux-arm64
hosts.yaml.example
compose.yaml
storctl-profiles.json
storctl-artifacts.example.json
README.md
docs/
examples/
```

`storctl-compose` embeds `storctl-linux-arm64`, but the standalone `storctl-linux-arm64` is included for single-host debugging.

## Notes

- `storctl-compose` always orchestrates 1823.
- Use standalone `storctl-linux-arm64` for CX7.
- Targets must allow root SSH login.
- `apply` never installs drivers; run `install-driver` first.
- The tool never reboots hosts automatically.
- Real driver packages are not stored in public releases.
