# gmp
用户增长

## 项目结构

- `gmp-core/`：Go 领域模型、接口、内存实现、业务回归与原有 CLI 演示。
- `frontend/`：前端展示层，目前包含独立、零框架依赖的 [运营流程沙盘 Demo](frontend/demos/operations-flow/README.md)。
- `docs/`：领域规格、产品设计与协作证据。

## 运营流程立体展示

全部是构造数据，不接真实渠道，也不依赖 `gmp-core` 服务。从仓库根目录启动静态服务：

```sh
python3 -m http.server 8080 --bind 127.0.0.1 --directory frontend/demos/operations-flow
```

浏览器打开 `http://127.0.0.1:8080`。如需单文件直接打开，使用 Demo 内的 `export.mjs`，见 [运行说明](frontend/demos/operations-flow/README.md)。

原命令行领域演示保持不变：在 `gmp-core` 下执行 `go run ./cmd/gmp-demo`。
