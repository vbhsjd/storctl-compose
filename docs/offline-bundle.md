# 离线 Bundle

离线 bundle 是给无网实验室使用的目录包，通常包含：

```text
storctl-linux-arm64
storctl-profiles.json
storctl-artifacts.json
driver-matrix.yaml
drivers/
checksums.txt
```

公开仓库不保存真实驱动包。请在内部环境准备 `drivers/` 目录，再运行：

```bash
./scripts/build-bundle.sh \
  --storctl ./dist/storctl-linux-arm64 \
  --profiles ./storctl-profiles.json \
  --matrix ./examples/driver-matrix.yaml \
  --drivers ./drivers \
  --out ./bundles \
  --name c4-openeuler22-aarch64
```
