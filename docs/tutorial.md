# storctl-compose 详细教程

这篇教程面向“我要批量把一批机器接入存储”的场景。单机接入逻辑由 `storctl` 完成，`storctl-compose` 负责把它组织成 Ansible 工作流。

## 1. 架构和边界

推荐分层：

```text
storctl
  单机命令。负责当前机器的驱动检查、VLAN、路由、QoS、NFS-RDMA 挂载和状态检查。

storctl-compose
  编排仓库。负责 inventory、profile、driver matrix、离线 bundle、Ansible playbook 和报告汇总。

Ansible
  批量执行器。负责 SSH、并发、复制文件、执行命令和收集结果。
```

`storctl-compose` 不保存真实驱动包、不保存真实生产 inventory、不实现 SSH 编排器。

## 2. 控制机准备

控制机需要：

- 能 SSH 到目标机器。
- 安装 Ansible。
- 有 `storctl-compose` 仓库。
- 有 `storctl-linux-arm64` 二进制。
- 有内部准备好的驱动包目录。

准备仓库：

```bash
git clone https://github.com/vbhsjd/storctl-compose.git
cd storctl-compose
```

准备目录：

```bash
mkdir -p dist drivers bundles reports
cp /path/to/storctl-linux-arm64 dist/
```

`drivers/` 目录只放在你的内部环境，不提交到公开仓库：

```text
drivers/
  storctl-artifacts.json
  SDK_LINUX-xxx-openEuler22.03SP4-aarch64.tar.gz
  MLNX_OFED_LINUX-xxx-openEuler24.03SP2-aarch64.tgz
```

## 3. 准备 inventory

复制示例：

```bash
cp examples/inventory.ini inventory.ini
```

每台机器至少写：

```ini
[storage]
node-39-149 ansible_host=80.5.17.113 storage_nic=enp23s0f1 nic_type=1823 storage_profile=c4
node-25-146 ansible_host=80.5.25.146 storage_nic=enp194s0f1np1 nic_type=cx7 storage_profile=c4

[storage:vars]
ansible_user=root
storctl_remote_bin=/usr/local/bin/storctl
storctl_artifact_dir=/root/storage_pkgs
storctl_profile_file=/etc/storctl/profiles.json
```

关键变量说明：

| Variable | 必填 | 说明 |
| --- | --- | --- |
| `ansible_host` | yes | SSH 管理 IP，也会传给 `storctl --mgmt-ip` |
| `storage_nic` | yes | 存储物理网卡，必须人工确认 |
| `nic_type` | yes | `cx7` 或 `1823` |
| `storage_profile` | yes | profile 名，例如 `c4` |
| `storctl_artifact_dir` | no | 目标机驱动目录，默认 `/root/storage_pkgs` |
| `storctl_remote_bin` | no | 目标机 storctl 路径，默认 `/usr/local/bin/storctl` |

不要把真实 inventory 提交到公开仓库。

## 4. 准备 profile

复制示例：

```bash
cp examples/storctl-profiles.json storctl-profiles.json
```

示例：

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
        {"server": "172.27.1.1", "export": "/Share", "mount_point": "/mnt/share"},
        {"server": "172.27.1.1", "export": "/Weight", "mount_point": "/mnt/weight"}
      ]
    }
  }
}
```

profile 适合放“同一个集群固定不变”的东西：

- VLAN ID。
- gateway。
- route table。
- MTU。
- 挂载点。
- 管理 IP 到数据网 IP 的三段映射。
- 是否启用 QoS。

inventory 适合放“每台机器不同”的东西：

- SSH 管理 IP。
- 存储物理网卡。
- 网卡类型。
- 使用哪个 profile。

## 5. 准备 driver matrix

`examples/driver-matrix.yaml` 是给人看的矩阵，建议维护为内部版本：

```yaml
drivers:
  - os_id: openEuler
    os_version_prefix: "22.03-LTS-SP4"
    arch: aarch64
    nic_type: "1823"
    file: "SDK_LINUX-xxx-openEuler22.03SP4-aarch64.tar.gz"
    sha256: "real-sha256"
    requires_repo: false
    tested: true
    notes: "validated on node-39-149"
```

`storctl-artifacts.json` 是给 `storctl install-driver` 用的机器清单。它应该放在 `drivers/` 目录里，并随驱动包一起进入离线 bundle。

公开仓库里只提交示例矩阵，不提交真实驱动包。

## 6. 构建离线 bundle

运行：

```bash
./scripts/build-bundle.sh \
  --storctl ./dist/storctl-linux-arm64 \
  --profiles ./storctl-profiles.json \
  --matrix ./examples/driver-matrix.yaml \
  --drivers ./drivers \
  --out ./bundles \
  --name c4-openeuler22-aarch64
```

输出：

```text
bundles/c4-openeuler22-aarch64.tar.gz
```

解开后大概是：

```text
c4-openeuler22-aarch64/
  storctl-linux-arm64
  storctl-profiles.json
  storctl-artifacts.json
  driver-matrix.yaml
  drivers/
  checksums.txt
```

校验：

```bash
mkdir -p tmp
tar -xzf bundles/c4-openeuler22-aarch64.tar.gz -C tmp
./scripts/validate-bundle.sh tmp/c4-openeuler22-aarch64
```

## 7. 执行前检查

先确认 Ansible 能连：

```bash
ansible -i inventory.ini storage -m ping
```

如果目标机没有 Python，可以先用 raw 做最小检查：

```bash
ansible -i inventory.ini storage -m raw -a "uname -a"
```

检查 playbook 语法：

```bash
ansible-playbook -i inventory.ini --syntax-check playbooks/00_facts.yml
ansible-playbook -i inventory.ini --syntax-check playbooks/10_copy_bundle.yml
ansible-playbook -i inventory.ini --syntax-check playbooks/20_install_driver.yml
ansible-playbook -i inventory.ini --syntax-check playbooks/30_plan.yml
ansible-playbook -i inventory.ini --syntax-check playbooks/40_apply.yml
ansible-playbook -i inventory.ini --syntax-check playbooks/50_check.yml
```

## 8. 推荐批量执行顺序

### 8.1 采集 facts

```bash
ansible-playbook -i inventory.ini playbooks/00_facts.yml
```

这一步不修改机器，用于看 OS、网卡、RDMA、systemd、命令是否存在。

### 8.2 复制二进制、profile 和驱动目录

```bash
ansible-playbook -i inventory.ini playbooks/10_copy_bundle.yml
```

默认复制：

- `./dist/storctl-linux-arm64` 到 `/usr/local/bin/storctl`
- `./storctl-profiles.json` 到 `/etc/storctl/profiles.json`
- `./drivers/` 到 `/root/storage_pkgs/`

### 8.3 安装驱动

```bash
ansible-playbook -i inventory.ini playbooks/20_install_driver.yml
```

这一步会执行：

```bash
storctl install-driver --nic-type {{ nic_type }} --artifact-dir {{ storctl_artifact_dir }}
```

如果某些驱动要求重启，先按实验室维护窗口处理重启，再继续后续步骤。

### 8.4 plan 预览

```bash
ansible-playbook -i inventory.ini playbooks/30_plan.yml
```

重点检查：

- `nic` 是不是存储网卡。
- `data-ip` 是否符合预期。
- `vlan` 是否正确。
- `mounts` 是否正确。

### 8.5 apply 接入

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml
```

默认 QoS 关闭。如果要启用：

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml -e storctl_qos=apply
```

建议先小批量执行：

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml --limit node-39-149
```

确认没问题后再扩大范围。

### 8.6 check 汇总

```bash
ansible-playbook -i inventory.ini playbooks/50_check.yml
```

每台机器的 JSON 会保存到：

```text
reports/<inventory_hostname>.json
```

汇总：

```bash
./scripts/collect-report.sh reports
```

## 9. 推荐上线节奏

第一批只跑 1 台：

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml --limit node-39-149
ansible-playbook -i inventory.ini playbooks/50_check.yml --limit node-39-149
```

第二批跑同类型 3 到 5 台：

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml --limit "node-39-149,node-39-150,node-39-151"
```

最后按机房或集群分批：

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml --limit storage
```

不要第一次就全量执行。存储网络里最容易出问题的是：网卡名、VLAN、三段映射、驱动版本、RDMA 服务端。

## 10. 常见批量问题

### SSH 断了

优先怀疑 `storage_nic` 写成了管理口。新版本 `storctl` 会用 `--mgmt-ip` 做保护，但 inventory 仍然应该人工核对。

检查 inventory：

```ini
node-1 ansible_host=80.5.17.113 storage_nic=enp23s0f1
```

`ansible_host` 是管理 IP；`storage_nic` 应该是 200G 存储口。

### 大量 FAIL driver

通常是 manifest 和 OS/arch/nic_type 不匹配。

在目标机上检查：

```bash
storctl facts --json
storctl validate-artifacts --artifact-dir /root/storage_pkgs
```

在控制机上检查 `drivers/storctl-artifacts.json` 和 `driver-matrix.yaml`。

### 部分机器 data-ip 不对

检查 profile 的 `third_octet_map`：

```json
"third_octet_map": {
  "17": 4,
  "21": 3
}
```

规则是：

```text
ansible_host 第三段 -> third_octet_map -> data-ip 第三段
ansible_host 第四段 -> data-ip 第四段
```

### RDMA 不通但 TCP 能挂

不要直接把 TCP 当成功。先看：

```bash
storctl check --json
rdma link
nfsstat -m
```

确实需要临时降级时，在 `storctl` 命令层显式传 `--allow-tcp-fallback`。当前 compose playbook 默认不打开这个选项，避免批量静默降级。

### QoS 要不要打开

默认不打开。只有交换机侧、存储侧和主机侧策略确认一致后，才批量启用：

```bash
ansible-playbook -i inventory.ini playbooks/40_apply.yml -e storctl_qos=apply
```

## 11. 验收标准

一批机器接入完成后，至少检查：

- `reports/*.json` 没有 `FAIL`。
- 没有非预期 `tcp_fallback_degraded`。
- 每台机器都有正确 `data0.<vlan>`。
- 每个挂载点是 `proto=rdma`。
- 重启后再跑 `playbooks/50_check.yml` 仍然通过。

## 12. 仓库维护建议

公开仓库维护：

- playbook。
- 示例 profile。
- 示例 driver matrix。
- 文档和脚本。

内部环境维护：

- 真实 inventory。
- 真实 `storctl-profiles.json`。
- 真实 `driver-matrix.yaml`。
- 真实驱动包和 `storctl-artifacts.json`。
- 每次上线后的 `reports/` 归档。
