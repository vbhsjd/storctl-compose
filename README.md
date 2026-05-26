# storctl-compose

[English](README.en.md)

`storctl-compose` 是 `storctl` 的批量接入工具：一个 Go 单二进制，通过 SSH/SFTP 登录目标机，复制 `storctl`、profile 和离线驱动包，然后自动尝试 1823 网卡完成挂载。

不需要 Ansible、sshpass、Python，也不需要手写存储网卡名。

## Fast Start

准备三样东西：

- `hosts.yaml`：机器 IP、root 账号、密码或 key。
- `compose.yaml`：profile、驱动目录、远端路径。
- `drivers/`：1823 离线驱动包和 `storctl-artifacts.json`。

```bash
cp examples/hosts.yaml hosts.yaml
cp examples/compose.yaml compose.yaml
cp examples/storctl-profiles.json storctl-profiles.json
mkdir -p drivers reports
```

`hosts.yaml`：

```yaml
hosts:
  - name: node-57-122
    ip: 80.5.21.122
    user: root
    password: "replace-me"
```

`compose.yaml`：

```yaml
profile: c4
profile_file: ./storctl-profiles.json
artifact_src: ./drivers
remote_bin: /usr/local/bin/storctl
remote_profile_file: /etc/storctl/profiles.json
remote_artifact_dir: /root/storage_pkgs
nic_type: "1823"
allow_tcp_fallback: true
qos: off
```

执行：

```bash
storctl-compose copy --hosts hosts.yaml --config compose.yaml
storctl-compose install-driver --hosts hosts.yaml --config compose.yaml
storctl-compose apply --hosts hosts.yaml --config compose.yaml
storctl-compose check --hosts hosts.yaml --config compose.yaml
storctl-compose report --report-dir reports
```

## Commands

```bash
storctl-compose copy             # 上传 storctl、profile、drivers
storctl-compose install-driver   # 显式安装 1823 驱动，不自动重启
storctl-compose apply            # 自动选择 1823 网卡并执行挂载
storctl-compose check            # 批量收集 storctl check --json
storctl-compose report           # 汇总 reports/
storctl-compose version --json
```

常用参数：

```bash
--concurrency 30          # 默认 30，最大 50
--limit node-a,node-b     # 只跑部分机器
--upgrade-firmware        # install-driver 时显式升级固件
```

## NIC Selection

`apply` 会自动筛选目标机上的 1823 网卡：

- 排除管理 IP 所在接口。
- 排除 `lo`、docker、veth、bridge、bond、team、VLAN 子接口。
- 只保留 `ethtool -i <nic>` driver 为 `hinic3` 或 `hinic` 的物理口。
- 按 carrier、100G+、无 IPv4、接口 up 排序。
- 逐个尝试，成功即停止。

失败尝试保存在：

```text
reports/<host>/attempts/<nic>.out
reports/<host>/attempts/<nic>.err
```

## Notes

- 只编排 1823；CX7 请直接用 `storctl` 单机命令。
- 目标机要求 root SSH 登录，第一版不做 sudo。
- `apply` 不会自动安装驱动，必须先显式跑 `install-driver`。
- 默认允许 TCP fallback；报告会标记 degraded，后续仍要排查 RDMA。
- 真实驱动包不进入公开仓库或公开 release。
- Release 二进制会内置 `storctl-linux-arm64`；源码开发构建时可在 `compose.yaml` 设置 `storctl_bin`。

更多细节见 [docs/tutorial.md](docs/tutorial.md)。
