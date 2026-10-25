# frontend

GMP 的前端展示层，与 `gmp-core` 领域模型分离。

目前包含独立的 [运营流程沙盘 Demo](demos/operations-flow/README.md)。它是零框架依赖的静态页面，不是完整的管理后台，不依赖 Go 服务，也不调用核心领域或真实渠道。

`demos/` 承载可独立运行的展示样例；未来如建设真实产品前端，再在本目录按实际需求确定框架、构建与 API 边界。本轮不提前引入 React、npm 工程或统一前端应用壳。
