# 离线 Bundle

离线 bundle 是给无网实验室使用的目录包，通常包含：

```text
storctl-compose
storctl-linux-arm64
storctl-profiles.json
storctl-artifacts.json
compose.yaml
hosts.yaml.example
drivers/
checksums.txt
```

推荐直接使用 GitHub Release zip，它已经包含 `storctl-compose`、独立 `storctl-linux-arm64` 和配置模板。公开仓库不保存真实驱动包，驱动仍由用户自己放进 `drivers/`。

如果内部还需要自己打包，可以运行：

```bash
./scripts/build-bundle.sh \
  --compose-bin ./dist/storctl-compose-linux-arm64 \
  --storctl ./dist/storctl-linux-arm64 \
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
