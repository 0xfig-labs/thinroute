# thinroute 优化方案

## 项目现状

- 602 次提交，697 个 Go 文件（291 测试文件），约 17.6 万行实现代码
- 22 个 LLM Provider 适配器，6 种路由策略，3 种存储后端
- 完整测试套件：e2e、integration、contract、perf、race
- 当前标签 v0.1.0，实际成熟度接近 v1.0

**核心问题**：从 GoModel 继承了大量企业级功能（多租户 IAM、审计日志、3 种数据库、Dashboard、Rate Limiting），与"个人 AI 网关"定位冲突，增加维护负担和新用户上手难度。

**最终形态**：单机、单用户、默认仅 loopback 暴露；无入站认证；SQLite 为唯一持久化后端；保留现有 OpenAI-compatible API 能力、轻量 usage 统计和 exact response cache；不保留 Dashboard、企业管控、正文审计或 semantic cache。全部改动一次性交付，不发布过渡版本。

---

## 1. 认证 — 删除所有入站认证

**目标**：thinroute 自身不要求任何认证。下游 Provider 的 API key 通过环境变量注入，与网关认证无关。

**设计**：将 `server.port` 改为完整监听地址 `server.listen`，默认值为 `127.0.0.1:52180`。本地调用无需认证；若配置为非 loopback 地址，启动时打印包含实际监听地址的明显警告。loopback 判断使用 `net.SplitHostPort`、`net.ParseIP` 和 `IP.IsLoopback()`，同时正确处理 `localhost`、IPv4 和 IPv6。Provider 调用的 API key 仍从环境变量读取，不受影响。

**删除**：

| 文件 | 操作 |
|------|------|
| `config/server.go` `MasterKey` 字段（第 25 行） | 删除 |
| `internal/server/auth.go`（`AuthMiddlewareWithAuthenticator`） | 删除 |
| `internal/server/http.go` 第 305-307 行 MasterKey/Authenticator 条件 | 删除——认证中间件永不注册 |
| `internal/server/Config.MasterKey` + `Authenticator` 字段 | 删除 |
| `internal/authkeys/`（整包，9 文件） | 删除 |
| `internal/control/routes.go` 第 59-63 行 `auth-keys` 路由 | 删除 |
| `internal/control/handler_authkeys.go` | 删除 |
| `internal/command/command.go` `authKeysCmd()`（第 567-685 行） | 删除 |
| `internal/app/app.go` `authKeyService` 初始化 | 删除注入 |
| `internal/server/auth_test.go`、`security_test.go` 中认证相关用例 | 删除或改写 |
| `config/control.go` `Token` 字段及 token 校验 | 删除 |
| `config/control.go` `Listen` 字段及 `CONTROL_LISTEN` | 改为 `control.listen` 字段，默认 `127.0.0.1:52181`；不通过独立环境变量覆盖 |
| `internal/command/command.go` `--control-token`、`CONTROL_TOKEN` 及 Authorization header 注入 | 删除 |
| control server 的 Bearer Token 中间件及相关测试 | 删除 |
| `server.port` 配置字段及 `PORT` 环境变量 | 替换为 `server.listen` |
| 端口冲突 | `net.Listen` 失败时 fatally exit，slog.Error 输出实际地址和错误原因 |
**新增**：

- `run/run.go` 启动时检查 API 监听地址：非 loopback 则 `slog.Warn("listening on non-loopback address without authentication", "address", addr)`
- 增加默认 IPv4 loopback、IPv6 loopback、`localhost`、通配地址和非法地址测试

**保留**：

- Provider API key 环境变量（`OPENAI_API_KEY`、`ANTHROPIC_API_KEY` 等）——这些是 thinroute 调用下游 Provider 时的凭据，属于出站而非入站

## 2. 一键启动 + CLI 可视化

### 启动简化

`thinroute` 不带参数直接启动服务器（当前已支持：无管理子命令时自动进入 serve 模式）。

### CLI 架构
**当前**：CLI 通过 HTTP 调 control API，需要网关先运行。

**改为**：保留精简版 control API，CLI 继续通过 HTTP 通信。control API 监听地址由 `config.yaml` 的 `control.listen` 字段（默认 `127.0.0.1:52181`）决定；CLI 读取同一配置文件确定连接地址。不提供 `--base-url` 参数。

### CLI 子命令

```bash
# Provider 状态（彩色表格）
thinroute providers status
# 输出：
# NAME        STATUS   LATENCY   MODELS  LAST REFRESH
# openai      🟢 ok    120ms     45      2m ago
# deepseek    🟢 ok    85ms      12      2m ago
# anthropic   🟡 slow  2500ms    8       2m ago
# gemini      🔴 down  -         -       2m ago

thinroute providers test openai   # 已有，改进输出格式

# 用量与成本统计
thinroute usage
# 输出：今日请求数、Token、成本估算、最常用模型 Top 5

thinroute usage --watch 2s
thinroute usage --json
thinroute usage --days 7

# 模型列表
thinroute models list

# 配置校验
thinroute config validate   # 已有，保留
```

### 删除的 CLI 命令

| 命令 | 理由 |
|------|------|
| `auth-keys` | 删除 authkeys 包 |
| `rate-limits` | 删除 ratelimit 包 |
| `logs tail` | 删除 live broker |
| `cooldown` | 内部状态，不需要 CLI 暴露 |
| `usage recalculate` | 删除 pricing overrides |
| `virtual-models` | 通过 config.yaml 管理，不通过 CLI CRUD |

### 表格/彩色输出

使用 ANSI escape 实现，不引入第三方 TUI 库。仅在 TTY 中启用颜色，遵循 `NO_COLOR`；`--json` 不输出颜色和 emoji：

```
\033[32m🟢\033[0m 绿色 = ok
\033[33m🟡\033[0m 黄色 = slow (latency > 1s)
\033[31m🔴\033[0m 红色 = down
```

---

## 3. 删除企业级功能

### 一次性删除

| 包/文件 | 文件数 | 功能 | 删除难度 |
|---------|--------|------|----------|
| `internal/authkeys/` | 9 | 托管 API Key IAM | 低 — 独立包 |
| `internal/ratelimit/` | 9 | 三范围限流 | 中 — 清理 server/ratelimit_support.go 和 app.go 注入 |
| `internal/live/` | 2 | Dashboard SSE 实时事件流 | 低 — Dashboard 删除后无消费者 |
| `internal/pricingoverrides/` | 13 | 自定义模型定价 | 低 — 独立包 |
| `internal/tagging/` | 4 | 请求头标签 | 低 — 同步删除相关统计维度 |
| `internal/storage/postgresql.go` | 1 | PG 后端 | 中 — 清理各 store 的后端选择 |
| `internal/storage/mongodb.go` | 1 | MongoDB 后端 | 中 — 同上 |
| 各包 `*_postgresql.go` `*_mongodb.go` | ~15 | PG/Mongo store 实现 | 中 — 机械删除 |
| `internal/responsecache/` 的 semantic cache 全部实现 | 多文件 | 语义响应缓存、embedder、vecstore factory/interface 及四种向量后端 | 中 |
| `config/cache.go` 的 semantic 配置与校验 | 多处 | semantic cache 配置、环境变量和后端校验 | 中 |
| `config/tagging.go` | 1 | 标签配置 | 低 — 同步删除 Config 字段 |
| `config/ratelimit*.go` | 3 | 限流配置 | 低 — 同步删除 Config 字段 |
| Redis 依赖 (`github.com/redis/go-redis/v9`) | — | response cache 的 Redis 后端 | 低 — response cache 仅保留内存后端；`go mod tidy` 自动移除 |

删除 PostgreSQL/MongoDB/Redis 后执行 `go mod tidy`，确保 `pgx`、`jackc/pgx`、MongoDB driver、`redis/go-redis` 以及不可达的 factory/config 分支一并移除。


### 保留并精简

| 包 | 处理 |
|----|------|
| `internal/auditlog/` | 移除正文/header 捕获、持久化 store/readers 和 control 查询；仅保留 server 仍需的请求上下文、错误、stream、route/failover enrichment |
| `internal/usage/` | 作为唯一轻量统计持久化链路，接收最终 metadata 并异步写 SQLite |
| `internal/conversationstore/` | 保留，维持 Conversations/连续对话兼容 |
| `internal/responsestore/` | 保留，维持 Responses 查询、取消和 input items |
| `internal/filestore/` | 保留，维持 Files/Batch 输入兼容 |
| `internal/batch/` | 保留，维持 OpenAI Batch API |
| `internal/batchrewrite/` | 保留，服务 Batch 请求改写 |

轻量记录仅包含：

```
request_id, timestamp, provider, model, status, latency,
input_tokens, output_tokens, cost, error, failover
```

上述 metadata 仅由 `internal/usage/` 持久化一次，不再建立第二套 audit 数据库记录。不得持久化用户 prompt、响应正文或敏感 header。成本使用内置模型价格目录计算；删除 pricing overrides 不影响基础成本估算。

### OpenAI API 能力边界

本次优化保留以下能力及其存储实现，不纳入删除范围：

```
internal/conversationstore/
internal/responsestore/
internal/filestore/
internal/batch/
internal/batchrewrite/
```

README 必须列出实际支持的 OpenAI-compatible 端点，避免将兼容范围表述为完整 OpenAI API 实现。

### control API 精简

`internal/control/routes.go` 删除以下路由组：

```
/auth-keys/*        — 删除 handler_authkeys.go
/audit/*            — 删除 handler_audit.go
/live/logs          — 删除 handler_live.go
/rate-limits/*      — 删除 handler_ratelimits.go
/tagging/*          — 删除 handler_tagging.go
/model-pricing-overrides/*  — 删除 pricing overrides 路由
/usage/recalculate-pricing  — 删除
```

保留现有的 control 路由，不新增 REST API：

```
/runtime/config, /runtime/refresh
/cache/overview
/usage/summary, /usage/daily, /usage/models
/providers, /providers/cooldown
/providers/:name/test, /providers/:name/refresh
/models, /models/categories
/virtual-models
```

### 保留的核心

| 包 | 处理 |
|----|------|
| `internal/usage/` | 复用精简后的 metadata 采集链路，异步写 SQLite，用于 CLI usage |
| `internal/providers/keyring.go` | 保留，单 key 不触发轮转逻辑，无运行时开销 |
| `internal/responsecache/` | 仅保留 exact/simple 缓存（内存）+ exchange + stream_cache；删除整个 semantic cache 和 Redis 后端 |
| `internal/control/` | 精简为上述保留路由；`cache/overview` 同步移除 semantic 字段 |
| `internal/observability/metrics.go` | 保留 Prometheus 指标 |

---

## 4. OpenAI/LiteLLM 兼容 + 集成指南

### 产品兼容承诺

本次优化保留现有 Responses、Files、Batch 和 Conversations 能力。README 必须逐项列出实际支持的端点，不使用“完整 OpenAI API 兼容”等宽泛表述。

### 验证清单

确定性 contract test（每次 CI 运行）：

- [ ] `GET /v1/models` 返回格式兼容 LiteLLM 的模型列表解析
- [ ] 非流式 `POST /v1/chat/completions`
- [ ] 流式 chunk 格式及 `data: [DONE]` 终止符
- [ ] 上游 401、429、5xx 的 OpenAI-compatible 错误映射
- [ ] model 不存在时的错误格式
- [ ] client timeout/cancel 能正确终止上游请求
- [ ] 响应中的 usage 字段正确
- [ ] 本地无认证模式接受 `Authorization: Bearer unused`
- [ ] provider/model 名称包含 `/` 时能正确路由

真实 Provider e2e 作为发布前或定期任务：普通 PR/定期任务缺少付费 key 时允许明确跳过；发布任务必须提供全部核心 Provider credentials。另用 LiteLLM 实际跑一轮 model list、非流式和流式 chat completion。

### 客户端集成文档（追加到 README.md）

````markdown
## Client Integration

thinroute 实现 OpenAI 兼容 API，可直接作为以下客户端的后端。

### LiteLLM Proxy

```yaml
# litellm_config.yaml
model_list:
  - model_name: gpt-5-mini
    litellm_params:
      model: openai/gpt-5-mini
      api_base: http://localhost:52180/v1
      api_key: unused
```

### Open WebUI

管理员设置 → 连接 → OpenAI API：
- URL: `http://localhost:52180/v1`
- Key: 任意非空值

### LibreChat

```yaml
# librechat.yaml
endpoints:
  custom:
    - name: thinroute
      apiKey: "unused"
      baseURL: "http://localhost:52180/v1"
      models:
        default: ["deepseek/deepseek-v4-flash"]
        fetch: true
```
````

---

## 5. 配置：config.yaml + 系统环境变量

**目标**：`config.yaml` 声明全部配置及 Provider credential 引用，真实凭据只存在于进程的系统环境变量中。不读取 `.env` 文件。

### 加载优先级

```text
--config PATH > THINROUTE_CONFIG > ./config.yaml > 内置默认值
```

### 改法

1. 增加显式 `--config`，并保留 `THINROUTE_CONFIG`，不再搜索 `config/config.yaml`
2. 删除反射式全配置环境变量覆盖；仅 YAML 中的 `${VAR}` / `${VAR:-default}` 展开和 `THINROUTE_CONFIG` 继续读取系统环境变量
3. 删除 `.env` 自动加载逻辑、`.env.template`/`.env.example` 和 README 中的 `.env` 指引
4. 从 `go.mod`/`go.sum` 移除 `github.com/joho/godotenv`
5. 所有非敏感配置（包括 Ollama `base_url`）只在 `config.yaml` 中声明
6. Provider 是否启用以 `config.yaml` 为准；需要凭据的 Provider 在配置中使用 `${PROVIDER_API_KEY}`，Ollama 等本地 Provider 可无 key 启用
7. 启动时若仍存在未展开的 `${VAR}`，立即报错并指出变量名，禁止把占位符原文当成 API key 发送
8. API 和 control 端口冲突时 fatally exit，slog.Error 输出实际地址和错误原因；不自动递增端口

系统环境变量由 shell、launchd/systemd、容器、CI 或密码管理工具注入：

```bash
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...
thinroute --config config.yaml
```

### config.yaml 完整示例

```yaml
# thinroute 配置文件
# Provider API key 的真实值由系统环境变量提供；此文件只保存变量引用：
#   export OPENAI_API_KEY=sk-...
#   export ANTHROPIC_API_KEY=sk-ant-...
#   export DEEPSEEK_API_KEY=sk-...
#   export GEMINI_API_KEY=...
#   export OPENROUTER_API_KEY=sk-or-...

server:
  listen: "127.0.0.1:52180"

control:
  listen: "127.0.0.1:52181"

# 按 Provider 配置模型列表和选项
providers:
  openai:
    api_key: "${OPENAI_API_KEY}"
    models:
      - gpt-5-mini
      - gpt-5
  deepseek:
    api_key: "${DEEPSEEK_API_KEY}"
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
  ollama:
    base_url: "http://localhost:11434/v1"

# 禁用指定模型的原生 reasoning
models:
  disable_reasoning:
    - deepseek/deepseek-v4-flash

# 虚拟模型：别名和负载均衡
virtual_models:
  - source: fast
    target: deepseek/deepseek-v4-flash
    strategy: cost
  - source: smart
    targets:
      - model: openai/gpt-5
        weight: 1
      - model: anthropic/claude-sonnet-4-20250514
        weight: 1
    strategy: round_robin

cache:
  model:
    refresh_interval: 3600

usage:
  enabled: true
  retention_days: 90

metrics:
  enabled: true
```

---

## 6. Provider 支持矩阵

### 等级定义

- **核心**：每次 CI 必须运行确定性 contract test；真实 Provider e2e 在定期任务或发布前运行。contract test 失败阻断合并；发布任务必须配置核心 Provider credentials，缺失、跳过或失败均阻断发布。
- **社区**：保证编译并通过共享 Provider contract；不承诺真实 API e2e。
- 普通 PR 和定期任务缺少真实 Provider key 时明确标记为 skipped，不得伪装为通过。
- README 的支持矩阵与 CI 配置同步维护。

| 级别 | Provider | 验收要求 |
|------|----------|----------|
| **核心** | OpenAI | PR contract；定期/发布前真实 e2e |
| **核心** | Anthropic | PR contract；定期/发布前真实 e2e |
| **核心** | Gemini | PR contract；定期/发布前真实 e2e |
| **核心** | DeepSeek | PR contract；定期/发布前真实 e2e |
| **核心** | OpenRouter | PR contract；定期/发布前真实 e2e |
| **核心** | Ollama | PR contract；发布前本地 e2e |
| **核心** | Compatible | 通用 OpenAI-compatible contract |
| 社区 | xAI, Groq, Fireworks, Azure, Bedrock, Vertex AI, Z.ai, MiniMax, MiMo, OpenCode Go, Oracle, vLLM, Bailian, Kimi Code | 编译 + 共享 contract |

---

## 7. 测试体系精简


### 事前基线

精简前必须记录精确基线，用于事后对比：

```bash
find . -name '*_test.go' | wc -l                    # 测试文件数
find . -name '*_test.go' | xargs wc -l | tail -1    # 测试代码总行数
go test ./... -count=1 -timeout 5m 2>&1 | tail -5   # 测试耗时
```

### 现状与目标

当前共有 291 个 `*_test.go`、约 9.66 万行测试代码，其中 250 个文件位于 `internal/`。大量测试继承自 GoModel，覆盖即将删除的企业功能、多存储后端矩阵和实现细节。

本次对全部测试进行一次性审查和精简，不按覆盖率或文件数机械保留。目标是删除无效和重复维护成本，同时保住核心行为回归能力。精简完成后重新检查所有保留测试的合理性和必要性，删除无用的。

### 直接删除

- 随 `authkeys`、ratelimit、live、tagging、pricing overrides、semantic cache、PostgreSQL、MongoDB 删除对应全部测试、fixture、golden、helper 和 integration setup
- 删除 Dashboard/admin、旧二进制名、旧配置环境变量和已移除 control 路由的测试
- 删除仅验证私有字段、构造步骤、默认值重复或第三方库行为的实现细节测试
- 删除 unit、integration、e2e 中验证同一分支且没有增加边界价值的重复用例
- 删除只为 GoModel 多租户、企业部署或多数据库兼容存在的 helper 和 test seam

### 合并与改写

- Provider 公共行为复用现有 shared contract；各 Provider 仅保留协议翻译、鉴权、流式解析和特有错误映射
- 相同输入/输出矩阵合并为 table-driven test，避免每个 case 单独建文件
- HTTP handler 优先通过公开路由测试，删除对内部调用顺序的断言
- config 测试仅保留加载优先级、strict YAML、`server.listen`、`${VAR}`/`${VAR:-default}` 展开、未解析占位符报错和非法配置边界
- audit/usage 测试改为验证单一 metadata 落库链路，以及正文和敏感 header 不落库
- Responses、Files、Batch、Conversations 只保留 API contract、关键状态转换和 SQLite 持久化测试，不再复制 store interface 的通用 CRUD 用例

### 必须保留的回归边界

```text
Models / Chat Completions / Responses / Files / Batch / Conversations contract
核心 Provider 请求翻译、响应翻译、streaming 和错误映射
路由、virtual models、retry、failover、cooldown
usage token/cost、SQLite 写入和 retention
exact response cache、exchange、stream cache
配置加载优先级与严格校验
client cancel、timeout、SSE [DONE] 和敏感数据不落库
```

### 最低回归要求

每个共享行为至少保留一个正常路径和一个关键错误路径。以下关键路径有最低数量要求：

| 路径 | 最低测试用例数 |
|------|----------------|
| Chat Completions 非流式 | 2（正常 + 错误） |
| Chat Completions 流式 + `data: [DONE]` | 2 |
| 每个核心 Provider 请求翻译 | 2（正常 + 错误） |
| virtual models 路由 | 2 |
| failover / retry | 1 |
| usage token/cost 写入 SQLite | 1 |
| 配置加载优先级与 `${VAR}` 展开 | 2 |
| client cancel / timeout | 1 |

不为代码行覆盖率制造无业务意义 case。
### 测试分层与 CI

| 层级 | 内容 | 运行时机 |
|------|------|----------|
| Unit | 纯转换、解析、路由、usage、config | 每次 PR |
| Contract | HTTP/OpenAI/LiteLLM 和 Provider shared contract | 每次 PR |
| Race | 共享状态、stream、cache、SQLite writer | 每次 PR 或独立必需 job |
| E2E | thinroute + mock/local Provider | 每次 PR |
| Real Provider E2E | 核心 Provider 真实 API | 定期及发布前 |
| Perf/Stress | 热路径基准和长时间压力 | 手动或定期，不阻断普通 PR |

### 验收状态

- [x] 已完成失效测试、企业功能和旧后端相关测试的清理。
- [x] `go.mod` 不包含 `godotenv` 或 `redis/go-redis`。
- [x] 未发现永久 skip；真实 Provider 缺少凭据时按约定显式跳过。
- [x] `go test ./...`、contract、race 和 mock/local e2e 全部通过。
- [ ] Real Provider e2e 仍需发布环境凭据，不在本地验收中执行。
- [ ] 精简前后测试数量和 CI 耗时基线未完整回填。
---

## 8. CI 修复

`.github/workflows/test.yml` 第 20 行，删除 `./cmd/thinroctectl` 引用：

```yaml
# 改前
- run: go build ./cmd/thinroute ./cmd/thinroctectl
# 改后
- run: go build ./cmd/thinroute
```

### 存储删减验收

- [x] `go.mod`/`go.sum` 不再包含直接使用的 PG、MongoDB、Redis 驱动。
- [x] 配置 schema 不再接受 PostgreSQL/MongoDB/Redis 字段。
- [x] factory 不保留不可达后端分支。
- [x] 文档、测试、OpenAPI 和构建标签无已删除后端残留。

### Semantic cache 删除验收

- [x] semantic middleware、embedder、vecstore interface/factory 和向量后端已删除。
- [x] semantic cache 配置、校验、环境变量、示例和测试已删除。
- [x] `semantic_hits` 及语义缓存 API 字段已从 OpenAPI 和 usage contract 删除。
- [x] response cache 仅保留 exact/simple、exchange 和 stream cache。
- Note: `PassthroughSemantic*` is intentionally retained as Provider protocol routing metadata; it is not semantic cache.
---

## 9. 一次性交付顺序

全部优化在同一个交付中完成，不发布过渡版本。以下仅表示同一实现过程中的依赖顺序；最终统一验收、统一交付。

1. 固定产品契约：保留 Models、Chat Completions、Responses、Files、Batch 和 Conversations API
2. 删除 Dashboard、IAM CRUD、rate limit、live、tagging、pricing overrides
3. 删除非 SQLite 后端（PG、MongoDB、Redis）和整个 semantic cache，完成依赖清理验收
4. 删除 authkeys、MasterKey、control.token 和全部入站认证；改用 `server.listen` 和 `control.listen`；新增非 loopback 监听警告和端口冲突 fatal exit
5. 将 audit 拆为轻量 usage recorder 与可删除的 body audit，再移除正文捕获
6. 精简 control API（删除 `/usage/log` 等路由）和 CLI，不新增 REST API
7. 收敛配置加载规则并更新示例、README
8. 记录测试基线，审查全部现有测试，删除失效测试并合并重复行为覆盖
9. 完成 LiteLLM contract、核心 Provider CI 和真实 e2e
10. 统一运行构建、单元测试、contract、race、e2e、`govulncheck` 和配置迁移检查，通过后一次性交付

## 不做

- 不考虑现有 v0.1.0 用户的升级兼容；旧 config.yaml 字段不保留兼容映射
- 不新增 Docker、Homebrew、Helm 等分发方式
- 不恢复 Admin Dashboard UI
- 不新增 Provider 或插件系统
- 不实现 OAuth/OIDC/TLS
- 不优化二进制体积（goreleaser `-s -w` 剥离后约 20-25MB，可接受）
