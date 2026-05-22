# 驱动矩阵

`driver-matrix.yaml` 是人维护的 OS/架构/网卡/驱动对应表。`storctl-artifacts.json` 是 `storctl install-driver` 读取的机器清单。

建议字段：

```yaml
drivers:
  - os_id: openEuler
    os_version_prefix: "22.03-LTS-SP4"
    arch: aarch64
    nic_type: "1823"
    file: "SDK_LINUX-xx-openEuler22.03SP4-aarch64.tar.gz"
    sha256: "replace-with-real-sha256"
    requires_repo: false
    tested: false
    notes: "example only; do not commit internal packages"
```
