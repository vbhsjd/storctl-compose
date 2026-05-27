# storctl-compose 教程

这篇教程面向“只提供 IP、账号、密码，然后批量接入 1823 NFS 存储”的场景。

## 1. 准备 release 目录

下载 release zip，解压后目录里已经有：

```text
storctl-compose
storctl-linux-arm64
hosts.yaml.example
compose.yaml.example
storctl-profiles.example.json
compose.yaml
storctl-profiles.json
storctl-artifacts.example.json
```

准备本地配置：

```bash
cp hosts.yaml.example hosts.yaml
cp compose.yaml.example compose.yaml
cp storctl-profiles.example.json storctl-profiles.json
mkdir -p drivers reports
cp storctl-artifacts.example.json drivers/storctl-artifacts.json
```

## 2. 编辑配置

`hosts.yaml` 只放登录信息：

```yaml
hosts:
  - name: node-57-122
    ip: 80.5.21.122
    user: root
    password: "replace-me"
```

`compose.yaml` 通常只需要确认路径：

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

`storctl-profiles.json` 写 VLAN、网关、IP 推导和挂载点；示例见仓库根目录 README。

## 3. 准备离线驱动

```text
drivers/
  storctl-artifacts.json
  SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz
```

可以从 release 里的 `storctl-artifacts.example.json` 复制后改真实文件名和 sha256：

```bash
cp storctl-artifacts.example.json drivers/storctl-artifacts.json
```

## 4. 执行

```bash
./storctl-compose copy
./storctl-compose install-driver
./storctl-compose apply
./storctl-compose check
./storctl-compose report
```

需要固件升级时：

```bash
./storctl-compose install-driver --upgrade-firmware
```

小批量：

```bash
./storctl-compose apply --limit node-57-122
./storctl-compose apply --limit "node-a,node-b,node-c"
```

## 5. 排障

失败尝试保存在：

```text
reports/<host>/attempts/<nic>.out
reports/<host>/attempts/<nic>.err
```

常见状态：

- `no_candidate_nic`：没有发现 1823 物理口，检查 `ethtool -i <nic>` 和连线。
- `driver_install_failed`：检查 `drivers/storctl-artifacts.json` 与 OS/SP/架构是否匹配。
- `tcp_fallback_degraded`：TCP 挂载成功但 RDMA 未成功，继续排查 RDMA、PFC/ECN 和服务端端口。

`storctl-compose` 永远按 1823 编排；CX7 请直接用 `storctl-linux-arm64` 单机排障。
