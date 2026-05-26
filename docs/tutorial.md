# storctl-compose Go 单二进制教程

这篇教程面向“只提供 IP、账号、密码，然后批量接入 1823 NFS 存储”的场景。

生产使用建议下载 Release 里的 `storctl-compose-linux-*`。Release 二进制已经内置 `storctl-linux-arm64`，`copy` 阶段会直接上传到目标机。源码开发构建时，如果没有替换内置 asset，需要在 `compose.yaml` 里增加：

```yaml
storctl_bin: /path/to/storctl-linux-arm64
```

## 1. 准备文件

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

## 2. 准备离线驱动

```text
drivers/
  storctl-artifacts.json
  SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz
```

`storctl-artifacts.json` 推荐写到 SP 级别：

```json
{
  "artifacts": [
    {
      "os_id": "openEuler",
      "os_version_prefix": "22.03-LTS-SP4",
      "arch": "aarch64",
      "nic_type": "1823",
      "file": "SDK_LINUX-17.12.5.0-openEuler22.03SP4-aarch64.tar.gz",
      "sha256": "replace-with-real-sha256",
      "requires_repo": false
    }
  ]
}
```

## 3. 执行顺序

```bash
storctl-compose copy --hosts hosts.yaml --config compose.yaml
storctl-compose install-driver --hosts hosts.yaml --config compose.yaml
storctl-compose apply --hosts hosts.yaml --config compose.yaml
storctl-compose check --hosts hosts.yaml --config compose.yaml
storctl-compose report --report-dir reports
```

需要固件升级时：

```bash
storctl-compose install-driver --hosts hosts.yaml --config compose.yaml --upgrade-firmware
```

小批量：

```bash
storctl-compose apply --hosts hosts.yaml --config compose.yaml --limit node-57-122
storctl-compose apply --hosts hosts.yaml --config compose.yaml --limit "node-a,node-b,node-c"
```

## 4. 自动选网卡规则

- 排除管理 IP 所在接口。
- 排除 `lo`、docker、veth、virbr、bridge、bond、team、VLAN 子接口。
- `ethtool -i` driver 必须是 `hinic3`/`hinic`。
- 候选按 carrier、100G+、无 IPv4、up 排序。
- 每个候选口逐个执行 `storctl apply`，成功即停止。

失败尝试保存在：

```text
reports/<host>/attempts/<nic>.out
reports/<host>/attempts/<nic>.err
```

## 5. 常见问题

`no_candidate_nic`：目标机没有发现 1823 物理口，检查 `ethtool -i <nic>` 和连线。

`driver_install_failed`：检查 `drivers/storctl-artifacts.json` 与 OS/SP/架构是否匹配。

`tcp_fallback_degraded`：TCP 挂载成功但 RDMA 未成功，后续仍要排查 RDMA、PFC/ECN 和服务端端口。
