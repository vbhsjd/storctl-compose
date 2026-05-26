# 离线 Bundle

离线 bundle 是给无网实验室使用的目录包，通常包含：

```text
storctl-compose
storctl-profiles.json
storctl-artifacts.json
compose.yaml
hosts.yaml
drivers/
checksums.txt
```

`storctl-compose` release 二进制已经内置目标机用的 `storctl-linux-arm64`。公开仓库不保存真实驱动包。请在内部环境准备 `drivers/` 目录，再运行：

```bash
./scripts/build-bundle.sh \
  --compose-bin ./dist/storctl-compose-linux-arm64 \
  --profiles ./storctl-profiles.json \
  --drivers ./drivers \
  --config ./compose.yaml \
  --hosts ./hosts.yaml \
  --matrix ./examples/driver-matrix.yaml \
  --out ./bundles \
  --name c4-openeuler22-aarch64
```

校验：

```bash
tmpdir="$(mktemp -d)"
tar -xzf ./bundles/c4-openeuler22-aarch64.tar.gz -C "$tmpdir"
./scripts/validate-bundle.sh "$tmpdir/c4-openeuler22-aarch64"
```
