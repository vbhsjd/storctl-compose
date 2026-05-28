# storctl-compose 用户使用说明

这篇文档给普通使用者看。管理员会提前准备好 `storctl-profiles.json`，用户只需要填写机器登录信息，然后执行 `copy`、`apply`、`report`。

项目地址：

- `storctl-compose`: https://github.com/vbhsjd/storctl-compose
- `storctl`: https://github.com/vbhsjd/storctl
- Release 下载页: https://github.com/vbhsjd/storctl-compose/releases

## 1. 准备文件

下载 `storctl-compose` release 包并解压：

```bash
unzip storctl-compose-v*-linux-arm64.zip
cd storctl-compose-v*-linux-arm64
```

目录里通常会有这些文件：

```text
storctl-compose
storctl-linux-arm64
hosts.csv.example
compose.yaml
storctl-profiles.json
README.md
docs/
examples/
```

把示例登录清单复制成正式文件：

```bash
cp hosts.csv.example hosts.csv
```

## 2. 填 hosts.csv

`hosts.csv` 只需要三列：

```csv
ip,password,user
141.61.50.185,你的密码,
141.61.50.141,你的密码,root
```

说明：

- `ip`：目标机器管理网 IP。
- `password`：SSH 登录密码。
- `user`：可不填，默认是 `root`。
- 如果使用普通用户，目标机器需要配置免密 sudo。

不要把真实 `hosts.csv` 上传到公开仓库。

## 3. 检查 profile

管理员会提前配置好 `storctl-profiles.json` 和 `compose.yaml`。

一般用户不需要改这两个文件，只需要确认 `compose.yaml` 里的 profile 名称是否正确：

```yaml
profile: c4
profile_file: ./storctl-profiles.json
artifact_src: ./drivers
allow_tcp_fallback: true
qos: off
```

如果不确定 profile 对不对，联系管理员确认。

## 4. 上传工具

先把工具和配置复制到目标机器：

```bash
./storctl-compose copy
```

如果驱动目录或网络较慢，可以加大超时：

```bash
./storctl-compose copy --timeout 60m
```

只操作某几台机器：

```bash
./storctl-compose copy --limit 141.61.50.185
./storctl-compose copy --limit 141.61.50.185,141.61.50.141
```

## 5. 挂载存储

执行挂载：

```bash
./storctl-compose apply
```

`storctl-compose` 会自动做这些事：

- 登录每台目标机器。
- 自动寻找 1823 存储网卡。
- 拉起本机网口。
- 配置 VLAN 和存储网络 IP。
- 挂载管理员配置好的 NFS 存储目录。
- 已经挂载成功的机器会自动跳过。

常见成功输出：

```text
OK   141.61.50.185 apply protocol=tcp already_mounted
OK   141.61.50.181 apply protocol=rdma selected-nic eth4
```

`protocol=rdma` 表示 RDMA 挂载成功。  
`protocol=tcp` 表示 TCP fallback 挂载成功，可以使用，但后续仍建议排查 RDMA。

## 6. 查看结果

查看终端汇总：

```bash
./storctl-compose report
```

默认报告只统计当前 `hosts.csv` 里的机器，旧记录不会混进来。

导出 CSV：

```bash
./storctl-compose report --csv result.csv
```

导出 Excel：

```bash
./storctl-compose report --xlsx result.xlsx
```

导出的字段是：

```text
ip,command,status,code,message,protocol
```

如果需要查看历史遗留记录：

```bash
./storctl-compose report --all
```

## 7. 常见失败

`auth_failed`

SSH 认证失败。检查 `hosts.csv` 里的账号和密码是否正确，或者目标机器是否禁止密码登录。

`ssh_timeout`

SSH 连接超时。检查目标机器是否在线、22 端口是否可达、网络和防火墙是否正常。

`connection_lost`

复制或执行过程中 SSH/SFTP 断开。可以单台重跑：

```bash
./storctl-compose copy --limit 目标IP --timeout 60m
./storctl-compose apply --limit 目标IP
```

`networkmanager_down`

目标机器 NetworkManager 没运行。需要在目标机上启动：

```bash
systemctl enable --now NetworkManager
```

`mount_failed`

NFS 挂载失败。查看详细日志：

```bash
cat reports/目标IP/last.json
ls reports/目标IP/attempts/
```

## 8. 关于驱动安装

当前推荐用户只执行：

```bash
./storctl-compose copy
./storctl-compose apply
./storctl-compose report
```

驱动安装流程还在开发和整理中。需要安装或升级 1823 驱动时，先联系管理员，不建议普通用户自行执行。

## 9. 查看更详细文档

更多说明见：

- README: https://github.com/vbhsjd/storctl-compose/blob/main/README.md
- 教程: https://github.com/vbhsjd/storctl-compose/blob/main/docs/tutorial.md
- 离线包说明: https://github.com/vbhsjd/storctl-compose/blob/main/docs/offline-bundle.md
- 单机工具 storctl: https://github.com/vbhsjd/storctl
