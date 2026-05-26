# 本机模拟测试

`storctl-compose/tests/sim/run.sh` 用来在没有真实实验室机器时跑 `storctl` 的控制面集成测试。它的目标是覆盖 90% 以上的可模拟场景：配置解析、OS/SP 识别、artifact 匹配、驱动安装命令、NetworkManager VLAN、QoS、RDMA/TCP fallback、多挂载点、持久化和状态检查。

`storctl-compose` 的 Go 批量逻辑（SSH/SFTP、候选网卡筛选、多候选尝试、报告汇总）由 Go 单元测试覆盖。模拟套件保留在 `storctl` 控制面这一层，避免重新引入旧版 Ansible/shell 自动选卡路径。

## 运行

在 `storctl` 仓库内嵌的 `storctl-compose` 目录中：

```bash
cd storctl-compose
./tests/sim/run.sh
```

如果 `storctl-compose` 是单独 clone 的仓库，需要指定 `storctl` 源码目录：

```bash
STORCTL_SOURCE_DIR=/path/to/storctl ./tests/sim/run.sh
```

测试会构建本地 `storctl` 二进制，然后生成模拟节点根目录：

```text
tests/sim/.work/nodes/
tests/sim/.work/reports/
```

失败时先看对应节点的：

```text
var/log/storctl-sim/commands.log
var/log/storctl-sim/shell.log
var/lib/storctl/state.json
etc/fstab
etc/systemd/system/
```

## 模拟机制

`storctl` 支持环境变量：

```bash
STORCTL_SIM_ROOT=/tmp/storctl-sim/node-1
```

设置后，以下路径会被重定向到模拟根目录：

```text
/etc/os-release
/etc/fstab
/etc/storctl/profiles.json
/etc/systemd/system
/run/systemd/system
/sys/class/net
/var/lib/storctl
/usr/local/sbin/storctl-qos.sh
```

模拟模式下不要求 root，并且 shell 命令会交给 `storctl-sim-sh` 记录，不执行真实 `/bin/sh -c` 写系统文件。

`tests/sim/fakebin` 提供这些假命令：

```text
nmcli rdma hinicadm3 ibdev2netdev mlnx_qos cma_roce_tos
findmnt nfsstat systemctl mount umount modprobe tar rpm dnf dracut ip ethtool
```

所有调用都会写入：

```text
$STORCTL_SIM_ROOT/var/log/storctl-sim/commands.log
```

## 覆盖场景

- openEuler `22.03-LTS-SP4` + 1823 + RDMA ready。
- openEuler `22.03-LTS-SP4` + 1823 + RDMA empty + TCP fallback。
- RDMA empty 且未开启 fallback 时失败。
- openEuler `24.03-LTS-SP2` + CX7。
- 管理网卡误选保护，命中后不执行 NM 修改。
- 已有 TCP 挂载时重新挂成 RDMA。
- 无 systemd 时写 `/etc/fstab`。
- SP 级 artifact 优先于宽泛 `22.03` artifact。
- 同等具体度 artifact ambiguous 失败。
- sha256 mismatch 失败。
- `doca-host*.rpm` 未传 `--allow-repo` 时失败。
- 已有 `data0.<vlan>` 时修正 VLAN parent 到当前候选网卡。

Go 单元测试额外覆盖：

- compose 自动选卡：多个 1823 候选口时第一个失败、第二个成功。
- compose 自动选卡：无 1823 候选口时报 `no_candidate_nic`。
- 管理 IP 所在网卡被排除。
- TCP fallback 成功时整体成功但报告 degraded。

## 边界

模拟器不验证：

- 真实内核模块是否加载。
- 真实固件是否刷写成功。
- RDMA 性能、PFC/ECN 交换机行为。
- NFS server 真实导出、ACL、端口和吞吐。

这些仍需要真机验收：

```bash
storctl install-driver --nic-type 1823 --artifact-dir /root/storage_pkgs
reboot
rdma link
storctl apply --profile c4 --nic enp23s0f1 --mgmt-ip 80.5.17.113 --nic-type 1823
storctl check --json
```
