# storctl simulator nodes

`tests/sim/run.sh` generates fresh node roots under `tests/sim/.work/nodes`.
The generated roots are intentionally not committed because they contain command
logs, reports, state files, fstab/unit output, and per-run artifact checksums.

Each node root is used through `STORCTL_SIM_ROOT`, so `storctl` sees paths such
as `/etc/os-release`, `/sys/class/net`, `/etc/fstab`, and `/var/lib/storctl`
inside that node instead of on the real host.
