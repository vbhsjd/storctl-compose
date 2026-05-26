# Ansible Workflow

推荐通过仓库根目录的 `./storctl-compose` 包装入口执行。它会读取 `hosts.yaml` 和 `compose.yaml`，生成临时 Ansible inventory，再调用 playbook。

```bash
./storctl-compose copy --hosts hosts.yaml --config compose.yaml
./storctl-compose install-driver --hosts hosts.yaml --config compose.yaml
./storctl-compose apply --hosts hosts.yaml --config compose.yaml
./storctl-compose check --hosts hosts.yaml --config compose.yaml
./storctl-compose report --report-dir reports
```

原则：

- 用户不再手写 `storage_nic`；`apply` 阶段自动发现 1823 候选网卡。
- 驱动安装是显式阶段，`apply` 不自动安装驱动。
- `ansible_host` 会作为 `storctl --mgmt-ip`，用于保护管理口并推导 data IP。
- 失败排障先看目标机 `/var/lib/storctl-compose/apply/` 和 `/var/lib/storctl/state.json`。
