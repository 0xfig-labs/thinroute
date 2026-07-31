# thinroute Agent Rules

## 项目边界

- 这是 Go 单体项目，模块路径为 `github.com/0xfig-labs/thinroute`，要求 Go 1.26.5（或兼容的 Go 1.26 版本）。
- 主要入口是 `cmd/thinroute`；启动生命周期在 `run`，配置加载在 `config`，依赖组装在 `internal/app`，HTTP/API 行为主要在 `internal/server`。
- Provider 适配、模型发现与路由位于 `internal/providers`；核心请求类型和流程位于 `internal/core`。修改公共请求行为前，先确认这些边界及其调用方。
- `config.example.yaml` 是配置示例；运行配置可来自 `config.yaml`、`THINROUTE_CONFIG` 和系统环境变量。不要把真实 Provider 凭据写入配置、源码、测试输出或提交。

## 产品与安全边界

- thinroute 默认只监听 `127.0.0.1:52180`，且网关本身不提供入站认证；将 `server.listen` 改为非 loopback 地址前必须明确评估网络暴露风险。
- control plane 默认独立监听 `127.0.0.1:52181`，不应与公开推理监听器混用，也不应让 control API 修改 `config.yaml`。
- Provider API key 属于出站凭据，只通过系统环境变量注入；不要新增入站认证与出站凭据混用的路径。
- 默认持久化是 SQLite；涉及存储、缓存、usage 或数据格式的改动，先确认现有配置和工厂边界，不要凭空增加后端或迁移兼容层。
- 不在日志、issue、测试失败输出或安全报告中包含 API key、请求正文、响应正文或本地敏感配置。

## 开发与验证

- 常用命令以 `Makefile` 为准：`make build`、`make test`、`make test-race`、`make test-e2e`、`make test-contract`、`make lint`。
- 快速验证优先使用与改动最接近的命令；涉及共享 API、配置、存储、启动流程或路由时，再运行 `go test ./... -count=1 -timeout 5m` 或对应 CI 检查。
- CI 的基础门槛还包括 `go build ./cmd/thinroute`、e2e、contract 和 perf guard；发布或跨模块改动不要只依赖单个包测试。
- 需要真实 Provider 的测试或 API 录制必须显式提供凭据，并避免把生成的响应、数据库和构建产物纳入提交；未配置凭据时不要伪造成功结果。
- 修改配置结构时至少验证默认配置、示例配置和严格配置解析；修改监听或健康检查时覆盖 loopback、非 loopback、非法地址和端口冲突等边界。
- 保持 Go 格式与现有 lint 约定；优先复用已有类型、工厂和测试辅助函数，不为单一调用新增抽象。

## 架构约定

- 依赖方向保持为入口/运行时 → 配置与应用组装 → internal 服务；底层 provider、storage 和 core 不应反向依赖 CLI 或 HTTP 启动入口。
- `internal/app` 负责组件初始化与生命周期收尾；新增组件必须纳入初始化失败时的回滚和正常 shutdown 路径。
- 请求路由、请求改写、provider 选择和响应缓存属于不同阶段；修改其中一阶段时，保留已有错误处理、超时、流式响应和 readiness 语义。
- 配置默认值的单一来源是 `config` 包；不要在 CLI、server 或示例文件中复制一套互相漂移的默认值。
- 只修改仓库根规则时不创建局部规则文件；若未来某个子目录形成独立且非默认的约束，再在该目录增加局部 `AGENTS.md`，不要复制本文件。
