# storctl-compose

[English](README.en.md)

`storctl-compose` 是给实验室批量接入 1823 NFS 存储用的工具。你只需要准备机器 IP、root 密码、驱动包和存储配置，它会通过 SSH/SFTP 登录每台机器，上传 `storctl`，自动找 1823 网卡，然后配置网络并挂载存储。

不需要 Ansible、sshpass、Python，也不需要提前知道存储网卡名。

## 推荐安装方式

用 GitHub Release zip，当成离线安装包使用。暂时不做 RPM/DEB。

原因很简单：

- 控制机可能是 macOS、Linux x86 或 Linux arm64，RPM/DEB 不一定合适。
- 目标机驱动包不能进公开仓库，必须用户自己放到 `drivers/`。
- zip 解压就能用，最适合无网实验室拷贝。

release zip 解压后应该看到：

```text
storctl-compose
storctl-compose.sha256
storctl-linux-arm64
storctl-linux-arm64.sha256
hosts.yaml.example
hosts.csv.example
compose.yaml.example
storctl-profiles.example.json
compose.yaml
storctl-profiles.json
storctl-artifacts.example.json
README.md
README.en.md
docs/
examples/
```

`storctl-compose` 是控制机上运行的批量工具。`storctl-linux-arm64` 是目标机上用的单机工具，也可以单独拷到某台机器上排障。

## 1. 准备目录

```bash
unzip storctl-compose-*.zip
cd storctl-compose-*
mkdir -p drivers reports
cp hosts.csv.example hosts.csv
cp compose.yaml.example compose.yaml
cp storctl-profiles.example.json storctl-profiles.json
cp storctl-artifacts.example.json drivers/storctl-artifacts.json
```

后面只需要改三个文件：

```text
hosts.csv
compose.yaml
storctl-profiles.json
```

驱动包放这里：

```text
drivers/
  storctl-artifacts.json
```

## 2. 填 hosts.csv

`hosts.csv` 写要接入存储的机器，三列就够：

```csv
ip,password,user
80.5.21.122,replace-me,
80.5.21.123,replace-me,root
```

`user` 可以不填，默认是 `root`。如果填普通用户，远端机器需要配置免密 sudo，因为 `copy/apply/install-driver` 要写 `/usr/local/bin`、`/etc/storctl`、NetworkManager 和挂载配置。

旧版 `hosts.yaml` 仍然兼容；需要密钥登录、端口等高级字段时可以继续用 YAML，并通过 `--hosts hosts.yaml` 指定。

## 3. 看 compose.yaml

大多数情况下不用改，只确认 `profile`、`artifact_src` 和远端路径。

```yaml
profile: c4
profile_file: ./storctl-profiles.json
artifact_src: ./drivers
remote_bin: /usr/local/bin/storctl
remote_profile_file: /etc/storctl/profiles.json
remote_artifact_dir: /root/storage_pkgs
allow_tcp_fallback: true
qos: off
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `profile` | 使用 `storctl-profiles.json` 里的哪个环境配置 |
| `artifact_src` | 本地驱动包目录，默认 `./drivers` |
| `remote_artifact_dir` | 上传到目标机的驱动目录 |
| `allow_tcp_fallback` | RDMA 失败时是否允许 TCP 挂载成功但标记 degraded |
| `qos` | 默认 `off`，需要时再改成 `apply` |

不需要写 `nic_type`。`storctl-compose` 固定只编排 1823。

## 4. 填 storctl-profiles.json

这里写存储网络和挂载点。先按下面模板改。

```json
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
      "qos": {
        "enabled": false
      },
      "mounts": [
        {
          "server": "172.27.0.50",
          "export": "/Share",
          "mount_point": "/mnt/share"
        },
        {
          "server": "172.27.0.50",
          "export": "/CommonRO",
          "mount_point": "/mnt/weight"
        }
      ]
    }
  }
}
```

最常改的地方：

| 字段 | 怎么填 |
| --- | --- |
| `vlan_id` | 实验室存储 VLAN |
| `gateway` | 存储网络网关 |
| `third_octet_map` | 管理网第三段到存储网第三段的映射 |
| `mounts` | NFS server、export 路径、目标挂载目录 |

IP 推导例子：

```text
管理 IP: 80.5.21.122
third_octet_map: "21": 3
prefix: 18
生成 data IP: 172.27.3.122/18
```

如果某台机器不符合这个规律，先不要批量跑，单独用 `storctl` 或单独 profile 处理。

## 5. 准备驱动包

把 1823 离线驱动包放进 `drivers/`：

```text
drivers/
  SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz
```

如果前面还没复制 artifact 模板，先复制：

```bash
cp storctl-artifacts.example.json drivers/storctl-artifacts.json
```

修改 `drivers/storctl-artifacts.json`，至少改这几项：

```json
{
  "os_id": "openEuler",
  "os_version_prefix": "22.03-LTS-SP4",
  "arch": "aarch64",
  "nic_type": "1823",
  "file": "SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz",
  "sha256": "replace-with-real-sha256",
  "requires_repo": false
}
```

生成 sha256：

```bash
sha256sum drivers/SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz
```

把输出的 hash 填到 `sha256`。

## 6. 先跑单台

先挑一台机器试跑：

```bash
./storctl-compose copy --limit node-57-122 --timeout 60m
./storctl-compose install-driver --limit node-57-122
```

如果驱动安装提示需要重启，手动重启目标机，然后继续：

```bash
./storctl-compose apply --limit node-57-122
./storctl-compose check --limit node-57-122
./storctl-compose report
```

确认没问题后再跑全部机器。

## 7. 批量执行

```bash
./storctl-compose copy
./storctl-compose install-driver
./storctl-compose apply
./storctl-compose check
./storctl-compose report
```

需要升级固件时才加：

```bash
./storctl-compose install-driver --upgrade-firmware
```

驱动目录很大、内网慢、`copy` 看起来卡住时，可以临时调大单机超时：

```bash
./storctl-compose copy --timeout 60m
./storctl-compose apply --timeout 30m
```

默认并发 30，最大 50：

```bash
./storctl-compose apply --concurrency 50
```

只跑几台：

```bash
./storctl-compose apply --limit node-a,node-b
```

## 8. 怎么判断成功

命令输出里看到类似：

```text
OK node-57-122 copy copied
OK node-57-122 install-driver driver installed
OK node-57-122 apply selected-nic enp23s0f1
OK node-57-122 check checked
```

汇总：

```bash
./storctl-compose report
```

默认汇总只看核心列：

```text
hosts success fail degraded driver_not_ready no_candidate no_link_ready reboot_required
```

如果有 `degraded`，说明 TCP fallback 成功了，但 RDMA 没成功，后续还要排查 RDMA。机器可读汇总用：

```bash
./storctl-compose report --json
./storctl-compose report --verbose
./storctl-compose report --csv result.csv
./storctl-compose report --xlsx result.xlsx
```

`--csv result.csv` 会输出所有机器，不只失败机器。字段只保留：

```text
ip,command,status,code,message,protocol
```

`protocol` 只有 `rdma` / `tcp` 两种值。想直接给 Excel 打开，推荐：

```bash
./storctl-compose report --xlsx result.xlsx
```

`xlsx` 第一行自带筛选，列宽也已经调大。

失败尝试日志在：

```text
reports/<host>/attempts/<nic>.out
reports/<host>/attempts/<nic>.err
reports/<host>/nic-probe/<nic>.json
reports/<host>/nic-probe/<nic>.hilink.txt
reports/<host>/nic-probe/<nic>.hilink-simple.txt
reports/<host>/nic-probe/<nic>.hilink-count.txt
```

## 常见问题

`ssh_failed`：检查 IP、root 密码、端口 22、目标机 SSH 配置。

`timeout`：单台机器某个阶段超过超时限制。`copy` 阶段通常是驱动目录太大或链路慢，可以用 `./storctl-compose copy --timeout 60m`。

`driver_install_failed`：检查 `drivers/storctl-artifacts.json` 的 OS/SP/架构、文件名和 sha256。

`no_candidate_nic`：目标机没有发现 1823 物理口，登录目标机跑 `ethtool -i <nic>` 看 driver 是否是 `hinic3` 或 `hinic`。

`no_link_ready_nic`：发现了 1823 物理口，但光模块、物理链路或速率不就绪。先看 `reports/<host>/nic-probe/`。

`all_candidate_nics_failed`：找到了 1823 网卡，但每个候选口都挂载失败，看 `reports/<host>/attempts/`。

`tcp_fallback_degraded`：TCP 挂载成功但 RDMA 未成功，继续检查 `rdma link`、交换机 PFC/ECN、服务端 NFS RDMA 端口。

小白排障命令：

```bash
hinicadm3 info
hinicadm3 hilink_port -i hinic0 -p 0
hinicadm3 hilink_port -i hinic0 -p 1
ethtool -i eth0
ethtool eth0
rdma link
```

`hinicadm3 -i` 通常填 `hinic0` 这种设备名，不是 `eth0/enp...`。先用 `hinicadm3 info` 看 `hinic0` 下面对应哪些 `NIC:<网卡名>`。

如果存储口被手动 `down`，`storctl-compose apply` 会尝试在目标机执行 `ip link set dev <nic> up`。如果是交换机端口 shutdown、光模块异常、线缆问题，工具只会报告，不会自动修复。

## 重要边界

- `storctl-compose` 只做 1823 批量接入。
- CX7 请用 release 里的 `storctl-linux-arm64` 单机操作。
- `apply` 不会自动安装驱动，必须先跑 `install-driver`。
- 工具不会自动重启机器。
- 真实驱动包不进入公开 release。
