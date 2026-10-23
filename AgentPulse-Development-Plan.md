# AgentPulse --- 开发计划与技术实施方案

> **产品定位**：Windows 优先、本地优先的 AI Agent
> 实时可观测性桌面应用。\
> **首个适配对象**：Codex Desktop。\
> **未来方向**：在稳定的核心数据模型上，逐步适配自研 Harness、Agent、MCP
> 工具及其他编码 Agent。\
> **文档状态**：Implementation Plan v1.0\
> **仓库名**：`AgentPulse`

------------------------------------------------------------------------

> **当前实施进度（2026-10-23）**：MVP 主体和 Windows 候选包已实现，Settings 可直接配置并保存 OTLP 端口，进程页可配置监控、采样周期及默认关闭的路径/命令行采集。会话页、工具分析页、进程树和指标分布字段已实现；当前 Codex 遥测未提供可靠的会话状态或项目路径字段。SQLite schema v14 已通过迁移验证，API 校验 Origin 和 bearer token 并使用令牌桶限流。后端全量测试、Flutter 分析与 30 项测试、Windows Release 构建、隔离包生命周期烟测和架构边界检查通过。真实 Codex 数据已迁移至 v14，历史迁移备份位于 `%APPDATA%\AgentPulse\backups\agentpulse.pre-v14-20261023-190616.db`。合成 10 万行性能检查达到目标：独立刷新 P95 431 ms、事件分页 P95 4 ms；并发查询下 10/100/1000 events/s 写入均达标，刷新 P95 565 ms。最新隔离 Windows E2E 向 UI 和数据库各验证 20 条日志，固定成功/失败命令与测试工作流产生 3 个 Span 和 3 个工具调用，3 个 PowerShell PID 明确关联；重试序号和 command/test 类型经 UI/API 核对。最新页面更新 P95 为 405 ms；七张完整窗口截图（1600×900）已复核，修复了会话详情在较矮视口下的时间线布局。该 E2E 通过进程页启用 codex.exe、500 毫秒采样，确认 4 条根进程记录后关闭监控，未保存路径或命令行。严格 OS 级离线验证按用户决定豁免，不记为技术通过。只读复核在 21:29 看到 schema 14 数据库约 2.85 GB，含 365 万条 Trace；这只是单日负载样本，长期保留和增长评估仍待多日观测。托管 CI 尚未运行，当前仓库没有已发布的 Git commit；更多真实 Codex 重试/任务类型字段样本也仍待采集。进程监控目前保持关闭。各阶段状态见[实施状态](docs/implementation-status.md)，架构对照见[架构审查](docs/architecture-review.md)。

------------------------------------------------------------------------

## 1. 项目目标

AgentPulse 用于回答 AI 编程任务执行期间的性能问题：

1.  当前 Agent 正在做什么？模型请求、工具调用、命令执行是否仍在进行？
2.  一次任务的总墙钟时间是多少？模型、工具和其他等待分别占多少？
3.  哪些工具调用耗时最长、失败最多、重试最多？
4.  输入/输出 Token、首 Token 延迟（TTFT）和可观测的生成速率如何变化？
5.  Shell、测试、构建和依赖安装等子进程消耗了多少时间、CPU 和内存？
6.  遥测是否完整？哪些数值是直接观测，哪些是推断或估算？

### 1.1 MVP 成功标准

-   Windows 上可安装、启动和卸载。
-   Flutter UI 可以启动、停止并监控 Go 后端。
-   Go 后端内置 OTLP/HTTP 接收服务，接收 Codex 实际导出的
    Logs、Metrics、Traces（以目标 Codex 版本实测为准）。
-   能够显示实时采集状态、任务列表、任务详情、工具调用时间线、Token
    指标和命令进程信息。
-   可以查看历史任务，并过滤时间、状态、工具类型和命令类别。
-   能区分任务墙钟时间、模型活动时间、工具调用时间、进程运行时间、重叠时间和未归因时间。
-   默认本地存储、默认不记录原始提示词，且不要求安装外部数据库或
    Grafana。
-   所有估算值有明确标签；缺失的数据不会伪装成 0。

### 1.2 非目标（MVP 不做）

-   不替代 Codex，不注入或修改其模型请求。
-   不读取或保存模型 API 密钥。
-   不截获任意 HTTPS 流量、不做 TLS 中间人代理。
-   不保证捕获每一个极短命的 Windows 子进程。
-   不把所有工具调用输出和提示词默认保存到数据库。
-   不搭建团队级云端监控服务。
-   不在第一版同时适配 Claude Code、Cursor、自研 Harness 等其他运行时。
-   不将推断出来的时间当成精确的模型推理耗时。

------------------------------------------------------------------------

## 2. 技术选型

  --------------------------------------------------------------------------------------------------------------
  层                      技术                                 选择理由 / 约束
  ----------------------- ------------------------------------ -------------------------------------------------
  桌面 UI                 Flutter Desktop（Windows）           一套 UI 组件、丰富的图表和桌面交互能力

  前端语言                Dart                                 页面、状态管理、模型与数据格式

  本地后端                Go                                   适合 HTTP 服务、并发事件管道、Windows
                                                               进程监控与单文件部署

  API                     Go `net/http` + REST + SSE           REST 用于查询，SSE
                                                               用于低复杂度实时推送；必要时再引入 WebSocket

  OTLP                    OTLP/HTTP Protobuf                   MVP 先支持 HTTP；gRPC 可在确有需要时增加

  OTel 数据类型           OpenTelemetry                        避免自行编写 Protobuf 解码逻辑
                          Protocol（OTLP）Protobuf 生成类型

  数据库                  SQLite + WAL                         单机、本地、无需数据库服务

  数据库访问              `database/sql` + SQLite 驱动         通过 Repository 隔离 SQL；优先评估纯 Go
                                                               驱动以简化 Windows 构建

  Windows 进程            `golang.org/x/sys/windows`，可评估   进程快照、PID/PPID、CPU、工作集内存；必要时扩展
                          `gopsutil`                           ETW

  图表                    Flutter 图表库（先评估 `fl_chart`    任务趋势、Token 速率、耗时分布
                          等）

  日志                    Go `slog`                            结构化本地日志，不记录敏感载荷

  配置                    TOML 解析库 + JSON 应用设置          Codex 配置与 AgentPulse 自身配置分开管理

  测试                    Go `testing`、Flutter test、集成测试 单元测试、协议 fixtures、UI 和打包验证

  CI                      GitHub Actions（Windows runner）     格式检查、测试、构建与打包
  --------------------------------------------------------------------------------------------------------------

### 2.1 关键架构决策

**Go 后端作为独立子进程运行，不通过 FFI 嵌入 Flutter。**

原因： - 后端崩溃时可以独立重启，避免整个 UI 一起退出。 - OTLP
接收、进程轮询和数据库写入不受 Flutter UI 帧率影响。 - Go
后端可以独立做集成测试和性能测试。 - 将来可以给 CLI、其他 UI
或自动化工具复用同一后端。

Flutter 启动后端时，应使用标准输入/输出或本地安全握手交换随机 API token
与动态 API 端口；不要把访问令牌写入命令行参数或日志。OTLP 端口默认绑定
`127.0.0.1`，启动失败时应明确报告端口冲突。

------------------------------------------------------------------------

## 3. 系统架构

``` text
┌──────────────────────────── Windows ─────────────────────────────┐
│                                                                   │
│  Codex Desktop                                                    │
│      │ OTLP/HTTP: logs / metrics / traces                         │
│      ▼                                                            │
│  ┌──────────────────── Go Backend ────────────────────────────┐  │
│  │  OTLP Receiver                                              │  │
│  │      ▼                                                      │  │
│  │  Decode → Normalize → Redact → Correlate → Analytics        │  │
│  │      ▲                              ▲                       │  │
│  │      │                              │                       │  │
│  │  OTel Events                  Windows Process Monitor       │  │
│  │                                     │                       │  │
│  │      └──────────────┬────────────────┘                       │  │
│  │                     ▼                                        │  │
│  │              SQLite (WAL mode)                               │  │
│  │                     │                                        │  │
│  │           REST API + Server-Sent Events                      │  │
│  └─────────────────────┬────────────────────────────────────────┘  │
│                        │ localhost                                  │
│                        ▼                                            │
│                 Flutter Desktop UI                                  │
└───────────────────────────────────────────────────────────────────┘
```

### 3.1 Go 后端内部数据流

1.  **接收**：校验 HTTP 路由、Content-Type、请求大小和认证信息。
2.  **解码**：将 OTLP Protobuf 转换为标准 OTel 消息对象。
3.  **归一化**：保留原始信号类型，同时映射出统一事件结构。
4.  **脱敏**：过滤提示词、命令参数、文件路径、环境变量和潜在密钥。
5.  **关联**：优先用 Trace/Span 关系和可用的 Codex 会话 ID
    关联事件；缺失时才做带置信度的推断。
6.  **持久化**：经有界队列批量写入
    SQLite；队列满、写入失败时产生可见的采集告警。
7.  **分析**：计算区间、速率、工具耗时、命令资源和数据质量。
8.  **推送**：向 Flutter 推送轻量变更通知；图表数据由 API
    批量读取，不逐事件刷新整页。

### 3.2 本地服务端口

  ----------------------------------------------------------------------------------------
  服务                    建议默认值              约束
  ----------------------- ----------------------- ----------------------------------------
  OTLP/HTTP               `127.0.0.1:4318`        默认值；设置页可持久化配置；保存前检查端口冲突，变更后重启接收器

  应用查询 API            动态本地端口            启动握手传递实际端口和 token

  SSE                     与应用查询 API 共用端口 由同一后端提供
  ----------------------------------------------------------------------------------------

所有服务只绑定回环地址。若 Codex 当前版本支持 OTLP
headers，则为导出请求设置随机
token；如果某信号的配置能力未经验证，不要假设认证配置有效。无论如何都必须保留
loopback 绑定、请求大小限制、速率限制和健康状态。

------------------------------------------------------------------------

## 4. 首要里程碑：验证 Codex 数据能力

**这是整个项目的第一道 Gate。未通过前，不应投入大量时间开发仪表盘。**

Codex 的遥测字段、导出信号和配置项会随版本变化；OTel
能提供什么，必须以用户实际安装的 Codex Desktop
版本为准。先做一个最小接收器，完成以下验证：

-   [x] 确认 Codex Desktop 读取的用户级配置路径。
-   [x] 确认 Logs、Metrics、Traces 各自是否能导出。
-   [x] 确认各信号使用的 endpoint、协议编码和 header 配置；当前配置使用各信号独立的 OTLP/HTTP binary endpoint，不配置 header，接收器只监听 loopback。
-   [x] 运行一项真实开发任务：提出问题、运行 shell
    命令、运行一次测试、完成最终总结。
-   [x] 保存脱敏后的 OTLP 样本，形成测试 fixtures。
-   [x] 列出实际存在的字段：会话
    ID、模型、请求起止时间、Token、工具名、工具状态、Trace/Span ID。
-   [x] 记录缺失字段、导出频率和聚合粒度。
-   [x] 验证重新启动 Codex 后采集恢复；Desktop 于 11:52:39 启动，三类信号从 11:55 起到达。
-   [x] 明确哪些指标不能从当前 OTel 数据源中可靠获得。

### 4.1 Codex 配置接入策略

Codex 的用户级配置通常位于：

-   Windows：`%USERPROFILE%\\.codex\\config.toml`
-   文档中的通用用户路径：`~/.codex/config.toml`

应用应该提供"检测配置 → 预览变更 → 备份 → 用户确认 → 应用变更 →
验证接收"的向导。

实施要求： - 不要用简单字符串替换覆盖整份 `config.toml`。 -
修改前备份原文件；失败时可以恢复。 - 尽可能只合并 OTel
相关键，保留其他设置。 - 不要自动启用原始提示词导出。 -
不能仅凭配置写入成功就显示"采集正常"；必须看到真实数据到达。 -
如果配置结构或字段名无法确认，显示可复制的配置建议和官方文档链接，不要静默修改。 -
首版不需要自动重启 Codex；明确提示用户重启应用使配置生效。

参考文档： - [Codex
配置基础](https://developers.openai.com/zh-Hans/docs/config-file/config-basic) -
[Codex
配置参考](https://developers.openai.com/ja-JP/docs/config-file/config-reference)

> 注意：官方配置参考会随版本更新。正式实现时应在目标 Codex Desktop
> 版本上验证具体 TOML schema，尤其是 metrics exporter 的 endpoint
> 配置，不能把示例直接视为所有版本通用。

------------------------------------------------------------------------

## 5. 功能范围与页面设计

### 5.1 Live Overview（实时总览）

显示： - 当前采集状态：Connected / No Data / Error。 -
当前活动会话和最近任务。 - 最近 1 分钟、5 分钟、1 小时的事件速率。 -
输入/输出 Token 统计（数据源支持时）。 -
请求延迟、TTFT（数据源支持时）。 - 正在运行的工具调用和命令进程。 -
最近错误、失败命令和重试。 - 最近一次收到 Logs、Metrics、Traces 的时间。

实时界面应使用明确的"无数据""部分信号不可用""采集失败"状态，不能用 0
代替缺失值。

### 5.2 Sessions（会话列表）

功能： - 按时间、状态、模型、项目路径（若可用）筛选。 -
显示任务墙钟时间、工具调用数、命令数、Token 总量和失败数。 -
分页或游标加载，避免一次加载所有历史事件。 -
允许查看"未能可靠归属会话"的事件。

### 5.3 Session Detail（任务详情）

页面区域： 1. 摘要卡片：墙钟时间、LLM
活动、工具执行、命令时间、未归因时间。 2. Waterfall
Timeline：按时间展示模型请求、工具调用、子任务和命令。 3. Tool
Calls：工具名称、开始/结束、耗时、状态、重试次数和可安全展示的摘要。 4.
Process Tree：进程层级、PID、可执行文件、CPU、内存、退出码。 5. Token
Analytics：输入/输出 Token、TTFT、可观测的速率。 6. Data
Quality：缺失事件、关联置信度、时间估算、进程采样间隙。

### 5.4 Tool Explorer（工具调用分析）

-   按工具名称聚合调用次数、成功率、失败率、P50/P95 耗时。
-   展示最慢调用、连续重试和重复调用。
-   支持按会话、工具类型、时间区间筛选。
-   将 OTel 工具 Span 与对应 OS 进程关联，但不把父 Span
    和子进程耗时重复相加。
-   默认不保存完整工具输入/输出；需要时由用户显式启用并设置保留期限。

### 5.5 Process Explorer（进程分析）

-   显示 PID、PPID、可执行文件、开始/结束时间、退出码。
-   采集 CPU 时间和工作集内存；I/O 指标按 Windows API 可用性逐步加入。
-   对 Shell、Git、构建、测试、Node、Python 等进程进行可配置分类。
-   区分直接观察的进程与根据时间/父子关系推断的关联。
-   对采样未捕获的短命进程显示监控局限。

### 5.6 Settings & Diagnostics（设置与诊断）

-   OTLP 端口与接收状态。
-   在设置页直接修改本地 OTLP 端口；变更后短暂重启接收器，并提示更新 Codex 配置。
-   Codex 配置状态、备份和恢复。
-   进程采样周期。
-   数据保留期限、数据库大小和清理操作。
-   隐私选项与脱敏规则。
-   导出脱敏诊断包。
-   显示后端版本、数据库 schema 版本和最后错误。

------------------------------------------------------------------------

## 6. 数据模型

使用 SQLite，开启 WAL。所有时间戳统一存为 Unix
纳秒整数（`INTEGER`）；前端显示时转换为本地时间。对缺失时间或估算时间必须有独立标记。

### 6.1 核心表

#### `sessions`

  字段                       类型           说明
  -------------------------- -------------- ------------------------------------
  `id`                       TEXT PK        AgentPulse 内部 UUID
  `source`                   TEXT           `codex`
  `external_session_id`      TEXT NULL      数据源实际提供的会话 ID
  `started_at_ns`            INTEGER        开始时间
  `ended_at_ns`              INTEGER NULL   结束时间
  `status`                   TEXT           `running/completed/failed/unknown`
  `model`                    TEXT NULL      可获取时记录
  `workspace`                TEXT NULL      可获取且用户允许保存时记录
  `correlation_method`       TEXT           `exact/inferred/unassigned`
  `correlation_confidence`   REAL NULL      0--1 置信度
  `created_at_ns`            INTEGER        记录创建时间

不要假设 Codex OTel 的每条事件都包含会话 ID。`external_session_id`
允许为空；推断会话应记录推断依据。

#### `turns`

  ----------------------------------------------------------------------------------
  字段                    类型                    说明
  ----------------------- ----------------------- ----------------------------------
  `id`                    TEXT PK                 内部 ID

  `session_id`            TEXT FK                 所属会话

  `external_turn_id`      TEXT NULL               数据源提供时填写

  `started_at_ns` /       INTEGER                 时间区间
  `ended_at_ns`

  `input_tokens` /        INTEGER NULL            缺失时为 NULL，不写 0
  `output_tokens`

  `cached_input_tokens` / INTEGER NULL            数据源支持时记录
  `reasoning_tokens`

  `ttft_ms`               REAL NULL               首 Token 延迟

  `generation_ms`         REAL NULL               可观测的生成区间

  `token_rate`            REAL NULL               输出速率

  `token_rate_kind`       TEXT                    `observed/estimated/unavailable`

  `data_quality`          TEXT                    `complete/partial/inferred`
  ----------------------------------------------------------------------------------

#### `telemetry_events`

保存经过脱敏、归一化的事件： -
`id`、`source`、`signal_type`、`event_name` -
`event_time_ns`、`received_at_ns` -
`trace_id`、`span_id`、`parent_span_id` -
`external_session_id`、`external_turn_id` -
`attributes_json`（只保存白名单属性） -
`payload_hash`（用于可选的重复事件检测） -
`redaction_status`、`schema_version`

默认不保存完整原始 OTLP 请求体。原始样本仅作为开发
fixtures，且必须脱敏。

#### `spans`

保存 Trace/Span 时间线： - `trace_id`、`span_id`、`parent_span_id` -
`name`、`kind`、`status_code` - `start_time_ns`、`end_time_ns` -
`attributes_json`（白名单） - `session_id`（可空） -
`correlation_method`、`correlation_confidence`

对 `(trace_id, span_id)` 建唯一索引；对 `start_time_ns`、`session_id`
建查询索引。

#### `tool_calls`

保存可识别的工具调用： - `id`、`session_id`、`turn_id` -
`external_tool_call_id`（可空） - `tool_name`、`tool_type` -
`start_time_ns`、`end_time_ns` - `status`、`retry_index` -
`input_summary`、`output_summary`（默认 NULL） -
`span_id`、`process_group_id`（可空） -
`correlation_method`、`correlation_confidence`

`tool_calls` 可以由 Span 或 Logs 派生。保留
`source_event_id`，便于回溯数据来源。

#### `processes`

-   `id`（内部进程实例 ID）
-   `pid`、`parent_pid`
-   `image_name`、`executable_path`（按隐私设置保存）
-   `command_line`（按独立隐私设置采集、脱敏后保存）
-   `started_at_ns`、`ended_at_ns`
-   `exit_code`
-   `cpu_time_ms`、`peak_working_set_bytes`
-   `session_id`（可空）
-   `association_method`、`association_confidence`

Windows PID 会复用，因此不能只用 PID 当作唯一主键。使用内部进程实例
ID，并将 PID 与启动时间组合用于识别实例。

#### `process_samples`

-   `process_id`、`sample_time_ns`
-   `cpu_time_ms`、`working_set_bytes`
-   `read_bytes`、`write_bytes`（支持时）
-   `sample_quality`

资源采样单独存表，避免频繁更新 `processes` 主表。

#### `metric_points`

用于存储可查询的指标点： - `id`、`metric_name`、`metric_type` -
`time_ns`、`numeric_value` - `unit`、`attributes_json` -
`temporality`、`aggregation_kind` - `source_resource_json`

Histogram 不能简单存成单个数值；首版可保留 OTLP
原始聚合结构的必要字段，并为常用直方图生成便于查询的汇总列。

#### `app_state`

保存最后接收时间、导出信号状态、数据库 schema
版本、配置状态、采集错误计数和最近错误摘要。

### 6.2 数据库索引与保留

建议索引： - `sessions(started_at_ns DESC)` -
`telemetry_events(event_time_ns)` - `spans(session_id, start_time_ns)` -
`spans(trace_id, span_id)` - `tool_calls(session_id, start_time_ns)` -
`processes(pid, started_at_ns)` -
`process_samples(process_id, sample_time_ns)` -
`metric_points(metric_name, time_ns)`

默认保留期建议为 14--30
天，允许用户调整。清理任务按批次删除旧事件并定期执行数据库维护；不要在
UI 线程执行大规模删除。

------------------------------------------------------------------------

## 7. 时间和 Token 分析规则

### 7.1 必须区分的时间

-   **Wall-clock duration**：任务结束时间减开始时间。
-   **LLM request duration**：可观测的模型请求区间。
-   **Tool span duration**：工具调用 Span 的区间。
-   **Process duration**：进程实际存活区间。
-   **LLM-only duration**：LLM 活动区间并集减去与命令区间重叠的部分。
-   **Command-only duration**：命令活动区间并集减去与 LLM
    区间重叠的部分。
-   **Overlap duration**：LLM 与命令区间交集。
-   **Unattributed
    duration**：任务墙钟时间中尚不能归属到已观测阶段的部分。

不要将所有 Span 的 duration 简单相加。父子 Span 可能重叠；并行 Agent
也会造成累计耗时大于墙钟时间。

### 7.2 Token 速率

优先展示以下指标： - 输出 Token 数。 - TTFT（首 Token
时间减请求发起时间）。 -
请求级有效输出速率：`output_tokens / request_duration_seconds`。 -
生成阶段平均速率：`output_tokens / generation_duration_seconds`，只有在生成阶段时间可观测时才展示。 -
Token 间隔：只有存在足够细粒度的流事件时间戳时才计算。

所有速率标记 `observed`、`estimated` 或 `unavailable`。不能用 API
请求的总耗时直接冒充纯模型解码时间；没有每
Token/每段流式时间戳时，也不能声称得到精确的实时逐 Token 速度。

### 7.3 数据质量

每个派生指标都应记录： - 使用的原始事件 ID。 - 计算规则版本。 -
数据来源。 - 是否有缺失事件。 - 是否为推断关联。 -
是否存在并行区间或时间戳异常。

------------------------------------------------------------------------

## 8. Windows 进程监控设计

### 8.1 MVP 策略

先做低侵入式轮询，不修改 Codex 的进程行为： - 默认每 500 ms
进行一次进程快照；设置中可调到 250--2000 ms。 - 发现 Codex
相关根进程，并记录 PID、PPID、进程名、启动时间。 -
跟踪可见的后代进程。 - 对存活进程采集 CPU 时间和工作集内存。 -
检测进程退出并尽可能读取退出码。 - 使用进程实例 ID，防止 PID
复用导致错误关联。

### 8.2 监控限制与后续增强

轮询可能漏掉在两个采样点之间启动并退出的短命进程。MVP 必须在 UI
中说明此限制。后续阶段再评估 ETW 进程启动/退出事件。

不要为了统计而强行将现有 Codex 进程加入 Job Object，也不要注入 DLL
或拦截任意进程。监控不能改变被监控任务的权限、沙盒或执行语义。

### 8.3 进程与工具调用关联

关联优先级： 1. 明确的 Span/工具调用 ID 与进程标识（若可获取）。 2.
明确的父子进程关系和时间区间。 3.
时间区间、命令类别和当前会话的启发式匹配。 4. 无法可靠匹配时保持未关联。

每次关联都记录 `association_method`
与置信度。不能只凭进程名就断言某命令属于某一轮 Agent 调用。

------------------------------------------------------------------------

## 9. 实时事件与 API 契约

### 9.1 查询 API（建议）

  Endpoint                                   用途
  ------------------------------------------ -------------------------------------------
  `GET /health`                              后端健康状态
  `GET /api/v1/status`                       OTel 信号状态、最后接收时间、进程监控状态
  `GET /api/v1/dashboard/summary?range=1h`   全局统计
  `GET /api/v1/sessions`                     会话列表、过滤、分页
  `GET /api/v1/sessions/{id}`                会话详情
  `GET /api/v1/sessions/{id}/timeline`       时间线数据
  `GET /api/v1/sessions/{id}/analysis`       会话时长、Token、工具分析与质量信息
  `GET /api/v1/sessions/{id}/processes`      进程树
  `GET /api/v1/events/unassigned`            未能可靠归属会话的事件，支持游标分页
  `GET /api/v1/tools/summary`                工具调用聚合
  `GET /api/v1/metrics/series`               指标时间序列
  `GET /api/v1/events`                       SSE 实时事件流
  `GET /api/v1/settings`                     应用设置
  `PUT /api/v1/settings`                     更新应用设置

API
应使用版本前缀。对时间序列和事件列表支持分页、时间范围及结果上限，避免大查询阻塞实时采集。

### 9.2 SSE 事件

建议事件类型： - `telemetry.status_changed` - `session.created` -
`session.updated` - `tool_call.started` - `tool_call.completed` -
`process.started` - `process.exited` - `alert.raised` -
`storage.backpressure`

SSE 推送的是轻量事件和实体 ID，不是反复推送完整历史数据。Flutter
收到通知后按需拉取最新数据。

### 9.3 API 安全

-   API 只监听回环地址。
-   使用随机访问令牌，并在启动握手中交给 Flutter；不放入命令行参数。
-   对写入 API 做严格的路由白名单。
-   校验 Origin/请求来源，限制请求大小和频率。
-   不允许通过 API 执行 shell 命令。
-   不暴露通用文件读取接口。
-   日志中不输出令牌、提示词、完整命令参数或原始 payload。

------------------------------------------------------------------------

## 10. 建议目录结构

``` text
AgentPulse/
├── README.md
├── AgentPulse-Development-Plan.md
├── CHANGELOG.md
├── CONTRIBUTING.md
├── docs/
│   ├── architecture.md
│   ├── architecture-review.md
│   ├── codex-integration.md
│   ├── telemetry-schema.md
│   ├── privacy-and-security.md
│   ├── decisions/
│   └── runbooks/
├── desktop/                         # Flutter Windows UI
│   ├── pubspec.yaml
│   ├── lib/
│   │   ├── main.dart
│   │   ├── app/
│   │   ├── routing/
│   │   ├── core/
│   │   │   ├── api/
│   │   │   ├── models/
│   │   │   ├── state/
│   │   │   └── theme/
│   │   ├── features/
│   │   │   ├── dashboard/
│   │   │   ├── sessions/
│   │   │   ├── timeline/
│   │   │   ├── tools/
│   │   │   ├── processes/
│   │   │   └── settings/
│   │   └── widgets/
│   └── test/
├── backend/                         # Go local backend
│   ├── go.mod
│   ├── cmd/agentpulse/main.go
│   ├── internal/
│   │   ├── app/
│   │   ├── api/
│   │   ├── otlp/
│   │   ├── telemetry/
│   │   ├── adapters/
│   │   │   └── codex/
│   │   ├── correlation/
│   │   ├── process/
│   │   │   └── windows/
│   │   ├── analytics/
│   │   ├── storage/
│   │   ├── config/
│   │   └── privacy/
│   ├── migrations/
│   └── tests/
│       └── fixtures/
├── scripts/
│   ├── dev.ps1
│   ├── build-windows.ps1
│   └── package-windows.ps1
└── .github/workflows/
    └── windows-ci.yml
```

### 10.1 目录边界

-   `otlp/` 只负责协议接收与解码，不直接计算业务指标。
-   `adapters/codex/` 负责把 Codex 特有字段映射到通用事件模型。
-   `telemetry/` 定义通用事件结构、验证与归一化。
-   `correlation/` 负责会话、Span、工具调用和进程的关联。
-   `analytics/` 只依赖通用领域模型，不依赖 Codex 原始字段。
-   `storage/` 是唯一直接访问 SQLite 的层。
-   `api/` 只负责输入校验、调用应用服务和序列化响应。
-   Flutter 只消费版本化 API，不直接访问 SQLite，也不依赖 Codex 原始
    OTLP 结构。

------------------------------------------------------------------------

## 11. 分阶段开发计划

按一名有 Flutter、Go 和 Windows 经验的开发者估算，MVP 约需 6--8 周；若
Codex
数据字段缺失或进程关联难度较高，应调整计划，不以固定日历为交付承诺。

### Phase 0 --- Codex 遥测可行性验证（2--3 天）

**任务**

- [x] 确认目标 Codex Desktop 版本与配置路径。
- [x] 构建最小 OTLP/HTTP 接收器。
- [x] 验证 Logs、Metrics、Traces 的真实导出行为。
- [x] 保存脱敏 fixtures。
- [x] 记录字段覆盖率和缺失能力。
- [x] 验证 endpoint 冲突、应用重启和断连恢复。

**验收**

- [x] 至少有一个完整真实任务的脱敏样本。
- [x] 每种信号都有“支持 / 不支持 / 尚未验证”的结论。
- [x] 对 Token 速率、TTFT、工具调用、会话 ID 的可用性有证据。
- [x] 如果关键数据不可得，已明确备选数据源和产品 UI 限制。

**Gate 0**：数据能力不清楚时暂停后续分析功能开发。

### Phase 1 --- 仓库、应用骨架与生命周期（3--4 天）

**任务**

- [x] 创建 Flutter Windows 项目和 Go module。
- [x] 建立 Flutter 启动、停止、异常退出检测和后端重启流程。
- [x] 设计安全启动握手，动态分配 API 端口。
- [x] 建立版本化 REST API 和健康检查。
- [x] 建立配置目录、结构化日志和错误码。
- [x] 加入 Windows CI：格式检查、单测、构建。

**验收**

- [x] Flutter 可以启动 Go 后端并确认 ready 状态。
- [x] 后端退出时 UI 显示错误，且可安全重启。
- [x] API 只监听 127.0.0.1。
- [x] 退出应用后不遗留子进程。

### Phase 2 --- OTLP Receiver 与 SQLite（4--6 天）

**任务**

- [x] 实现 /v1/logs、/v1/metrics、/v1/traces。
- [x] 实现 Protobuf 解码、内容类型和大小限制。
- [x] 实现有界队列、批量写入与背压计数。
- [x] 创建数据库迁移和核心表。
- [x] 记录最后收到信号时间和解析错误。
- [x] 加入可重复运行的 OTLP fixture 测试。

**验收**

- [x] 三种信号的合法样本可以解析并持久化。
- [x] 非法 Protobuf、过大请求和未知字段不会导致后端崩溃。
- [x] 数据库暂时不可用时会告警，且不会无限增长内存。
- [x] 应用重启后历史事件仍可查询。
- [x] 不默认保存原始 prompt 或完整工具输入/输出。

### Phase 3 --- Codex Adapter 与会话模型（4--5 天）

**任务**

- [x] 建立 Codex 专用字段映射层。
- [x] 归一化模型请求、Token 统计、工具调用和错误事件。
- [x] 提取实际存在的 session/turn 标识。
- [x] 建立 sessions、turns、spans 和 tool_calls。
- [x] 为缺失标识建立“未关联 / 推断关联”路径。
- [x] 为每种派生事件保存来源 ID。

**验收**

- [x] 使用真实 fixture 可生成会话列表和调用时间线。
- [x] 缺失的 Token 和 ID 保留为 NULL，而非 0 或虚构 ID。
- [x] 未关联事件可以单独查询。
- [x] Codex 字段变化会被测试捕获，不会静默生成错误指标。

### Phase 4 --- Windows Process Monitor（4--6 天）

**任务**

- [x] 实现进程快照采集和根进程配置。
- [x] 追踪 PID、PPID、进程启动时间和结束时间。
- [x] 采集 CPU 时间、工作集内存和退出码（可获取时）。
- [x] 保存资源采样点。
- [x] 实现命令类别规则和用户可编辑配置。
- [x] 建立进程与工具调用的关联规则及置信度。

**验收**

- [x] 可以识别已知测试程序和长时间运行的子进程。
- [x] PID 复用不会覆盖旧进程记录。
- [x] 采样周期可配置，异常不影响 Codex 本身。
- [x] 对短命进程漏采有明确说明。
- [x] 不注入进程、不修改 Codex 权限和沙盒。

### Phase 5 --- 分析引擎（4--5 天）

**任务**

- [x] 实现区间并集、交集和未归因时间计算。
- [x] 处理父子 Span、重叠调用和并行命令。
- [x] 实现 Token 统计、TTFT 和速率计算。
- [x] 计算工具调用次数、失败率和耗时分位数。
- [x] 为派生指标附加计算规则版本和数据质量。
- [x] 编写边界测试：零长度、缺失结束时间、乱序、重复和重叠事件。

**验收**

- [x] 墙钟时间不会因并行命令被重复累加。
- [x] 父子 Span 不会重复计入同一类独占时间。
- [x] 请求级速率与生成阶段速率清楚区分。
- [x] 不可计算的指标显示 unavailable，而非错误的 0。
- [x] 所有统计可通过 fixtures 重现。

### Phase 6 --- Flutter 完整分析界面（6--8 天）

**任务**

- [x] 完成 Dashboard、Sessions、Session Detail、Tool Explorer、Process Explorer、Settings。
- [x] 实现瀑布时间线和缩放/选择/详情面板。
- [x] 实现 Token 速率和耗时趋势图。
- [x] 实现 SSE 实时状态更新。
- [x] 实现过滤、分页、空状态、错误状态和加载状态。
- [x] 展示指标来源和质量标签。
- [x] 对高频事件做 UI 批处理，避免重绘风暴。

**验收**

- [x] 实时事件到 UI 的延迟目标 P95 < 1 秒；最新隔离 Windows 集成测试测得 20 条当前时间 OTLP 事件的 UI 更新 P95 为 405 ms。
- [x] 选择一个任务即可同时查看 LLM、工具和命令时间线。
- [x] 历史查询不会阻塞 OTLP 接收；10 万条历史数据下，并发查询时 10/100/1000 events/s 均达目标。
- [x] UI 能明确区分“没有数据”和“值为 0”。
- [x] 时间线支持并行和重叠区间，而非只按顺序排列。

### Phase 7 --- 配置向导、隐私与打包（4--6 天）

**任务**

- [x] 实现 Codex 配置检测、变更预览、备份、应用和恢复。
- [x] 实现端口冲突处理和采集健康诊断。
- [x] 实现数据库清理、数据导出和应用日志清理。
- [x] 生成 Windows 安装包，验证安装、升级、卸载。
- [x] 处理后端退出、数据库锁定和应用强制关闭。
- [x] 用户决定豁免严格 OS 级离线隔离（2026-10-23）；该项未作技术验证，不记为通过。

**验收**

- [x] 安装后可按向导配置并接收真实数据。
- [x] 修改配置前有备份，恢复路径可验证。
- [x] 卸载行为对用户数据有明确说明，不静默删除历史数据。
- [x] 端口冲突、无数据和配置错误都有可操作提示。
- [x] 默认设置不导出原始 prompt，不上传遥测到云端。

------------------------------------------------------------------------

## 12. 测试策略

### 12.1 单元测试

-   OTLP 解码、属性映射与未知字段处理。
-   脱敏规则和白名单。
-   区间并集/交集、时间差、重叠区间。
-   Token 速率和缺失数据处理。
-   PID 复用与进程生命周期。
-   事件去重和乱序处理。
-   SQLite 迁移与事务回滚。

### 12.2 集成测试

-   用脱敏 OTLP fixtures 发送到本地 receiver。
-   检查三类信号能否被持久化并由 API 查询。
-   进程监控运行期间持续写入遥测，确认不会互相阻塞。
-   后端重启后数据库和历史数据可恢复。
-   API token 缺失/错误时请求被拒绝。
-   OTLP 队列满、数据库写入失败时，健康状态能够反映问题。

### 12.3 Windows E2E

构造一条固定测试工作流： 1. 启动一个测试任务。 2.
运行一个持续数秒的命令。 3. 运行一个失败命令。 4. 运行测试程序。 5.
完成任务。 6. 验证会话、Span、工具调用、进程和耗时数据。 7. 验证 UI
时间线与数据库查询一致。

当前自动化验证由 `scripts/verify-windows-e2e.ps1` 驱动隔离的 Windows 应用，发送 20 条 OTLP 日志并核对 UI/API/数据库计数，再执行成功命令、预期失败命令和 PowerShell 测试程序。最新页面更新 P95 为 405 ms；3 个唯一工具 Span、3 个调用、1 个失败、command/test 类型、retry_index=1 和 3 个明确进程关联均经 UI/API/数据库核对。进程页的 codex.exe、500 毫秒采样设置经 UI 保存，路径和命令行保持关闭，监控在结束时关闭。E2E 现在截取并复核七张 1600×900 页面图，详情弹窗及关闭操作均落在窗口内。截图保存在 `dist/e2e-visual-review`。严格 OS 级离线隔离按用户决定豁免，不记为验证通过。托管 CI 尚未运行，长期保留/增长及更多真实 Codex 重试与任务类型样本仍待补齐。

### 12.4 性能测试目标（初始目标，实测后调整）

-   常态采集不应明显影响 Codex 执行。
-   OTLP 接收使用有界内存队列。
-   以 10 events/s、100 events/s 和突发 1000 events/s 进行压力测试。
-   对至少 10 万条历史事件执行分页和时间范围查询。
-   进程采样默认 500 ms，并允许降低采样频率以减少开销。
-   后端退出时尽量完成队列 flush，但设置最大退出等待时间。

可运行 `cd backend; go run ./tools/perfcheck -events 100000 -samples 20 -rate-duration 2s` 重现合成负载检查。工具会创建并在退出时删除临时 SQLite 数据库，不会打开用户数据库；输出接收速率、历史事件分页和并发仪表盘刷新分位数。

------------------------------------------------------------------------

## 13. 隐私与安全要求

AgentPulse
监控的内容可能包含代码仓库路径、命令参数、工具输入/输出和提示词，因此隐私必须作为核心功能设计。

-   默认不记录原始提示词。
-   默认不持久化完整工具输入/输出和命令行参数。
-   只持久化分析所需的白名单属性。
-   对路径和命令行提供独立开关及脱敏策略。
-   API 与 OTLP receiver 仅绑定 loopback。
-   本地 API 使用随机令牌；令牌不写入命令行或日志。
-   对请求大小、频率、队列容量和数据库写入速率设置上限。
-   不允许从 UI 触发任意命令执行。
-   数据库文件权限遵循当前用户权限，不创建对所有用户可读的目录。
-   提供一键清理历史数据和导出脱敏诊断包。
-   默认不进行遥测上报、崩溃日志上传或自动更新检查；若未来加入，应单独告知并征得用户同意。

------------------------------------------------------------------------

## 14. 未来适配其他 Agent 的扩展设计

虽然 MVP 只适配 Codex，但通用架构边界应在第一版就建立，避免以后把 Codex
字段写进核心模型。

### 14.1 Source Adapter 接口

所有数据源经过 adapter 转换为通用事件：

``` text
Codex OTLP ───────┐
Future Agent A ───┼──> Source Adapter ──> Canonical Event Model
Custom Harness ───┤                           │
MCP Instrumentation┘                           ▼
                                      Correlation / Analytics
```

概念接口示例（仅表达边界，不是最终 Go API）：

``` go
type SourceAdapter interface {
    SourceID() string
    Normalize(input RawTelemetry) ([]CanonicalEvent, error)
}
```

通用模型至少包括： - `Session` - `Turn` - `Agent` - `ModelRequest` -
`ToolCall` - `Process` - `Span` - `MetricPoint` - `ErrorEvent`

每个事件保留
`source`、`external_id`、`observed_at`、`correlation_method` 和
`data_quality`。来源特有属性放在有版本的扩展字段中，不污染通用分析逻辑。

### 14.2 适配路线

1.  **MVP：Codex Desktop OTLP** --- 完成基础采集、工具分析和进程关联。
2.  **下一阶段：Codex Session/Trace 补充** --- 只有在 OTel
    缺少必要字段且本地数据格式可维护时才添加；使用版本化解析器。
3.  **自研 Harness SDK** --- 定义可选的轻量事件协议，直接输出
    session、agent、tool 和 parent/child 关系。
4.  **MCP 工具调用** ---
    以标准工具调用事件补充工具输入、输出、耗时和错误，仍默认脱敏。
5.  **其他 Agent** --- 逐个实现 adapter，并为每个适配器维护独立 fixtures
    和兼容性测试。

不应在 MVP 阶段同时为多个 Agent 设计大量专用分支。先把 Codex
的采集链路、规范化模型和时间线分析做可靠，再扩展来源。

------------------------------------------------------------------------

## 15. 风险清单

  -----------------------------------------------------------------------------------------
  风险                     影响                    应对
  ------------------------ ----------------------- ----------------------------------------
  Codex                    Token 或调用数据缺失    Phase 0 真实验证；显示能力矩阵
  某版本不导出目标信号

  OTel 缺少会话 ID         无法精确关联            保留未关联事件；按
                                                   Trace/时间/进程推断并标置信度

  进程轮询漏掉短命进程     命令列表不完整          明确限制；后续评估 ETW

  父子 Span 重叠           耗时重复计算            区间并集/交集和独占时间计算

  数据库写入慢             事件丢失或 UI 延迟      有界队列、批量写入、背压指标

  Codex 配置格式变化       自动配置失败            版本验证、预览/备份/回滚，不做盲目替换

  事件属性包含敏感信息     隐私泄露                白名单、脱敏、最小化存储

  端口冲突或本地请求伪造   无法采集或事件注入      明确端口状态、loopback、随机令牌、限流

  UI 事件过密              卡顿                    批量推送、采样、分页、图表降采样
  -----------------------------------------------------------------------------------------

------------------------------------------------------------------------

## 16. 项目管理与交付流程

每个 Phase 使用独立任务清单，并要求在进入下一阶段前通过验收。

仓库以 `AgentPulse-Development-Plan.md` 作为唯一里程碑来源，并维护： -
`docs/architecture.md`：架构和模块边界。 -
`docs/decisions/`：架构决策记录（ADR）。 -
`docs/telemetry-schema.md`：通用事件 schema 与版本策略。 -
`docs/codex-integration.md`：Codex 配置、信号能力和版本兼容。 -
`docs/privacy-and-security.md`：隐私、数据保留与本地服务安全。 -
`CHANGELOG.md`：用户可见变更。 -
`docs/runbooks/`：构建、测试、打包、恢复和故障诊断。

开发流程： 1. 开始每个 Phase 前，明确范围、验收条件和非目标。 2.
先写测试和脱敏 fixtures，再实现解析/关联逻辑。 3.
每次合并前运行格式检查、单测、集成测试和 Windows build。 4.
验收失败时先修复，不带着已知问题进入下一阶段。 5. 更新文档、数据 schema
版本和变更记录。 6. 任何会改变数据格式的变更都通过 SQLite migration
完成。

------------------------------------------------------------------------

## 17. 首个 Sprint 的具体任务

第一周不做完整 UI，优先证明数据链路可行。

### Sprint 1：Codex → Go Receiver → SQLite

-   [x] 创建 `AgentPulse` 仓库。
-   [x] 建立 `backend/` Go module。
-   [x] 实现 `/health`。
-   [x] 实现 OTLP `/v1/logs`、`/v1/metrics`、`/v1/traces`
    的最小接收路径。
-   [x] 使用脱敏 fixture 测试 Protobuf 解码。
-   [x] 在目标 Windows 机器上配置 Codex 指向本地 receiver。
-   [x] 执行包含模型响应、工具调用和命令的真实开发任务。
-   [x] 保存接收到的信号类型、属性名和时间戳样本。
-   [x] 输出一份 `docs/codex-integration.md` 能力矩阵。
-   [x] 明确哪些指标能直接计算，哪些必须估算，哪些暂时无法得到。

**Sprint 1 完成定义：** 有证据证明 Codex 实际发出的遥测能被本地 Go
服务接收、解码和存储。若该目标尚未达成，不开始实现完整仪表盘。

------------------------------------------------------------------------

## 18. 参考资料

以下链接用于实现时核对当前版本的行为；版本相关设置必须在目标 Windows
环境中复验。

-   [Codex
    配置基础](https://developers.openai.com/zh-Hans/docs/config-file/config-basic)
-   [Codex 配置参考（含 OTel
    配置键）](https://developers.openai.com/ja-JP/docs/config-file/config-reference)
-   [OpenTelemetry Protocol](https://opentelemetry.io/docs/specs/otlp/)
-   [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
-   [Flutter
    Desktop](https://docs.flutter.dev/platform-integration/desktop)
-   [Go 官方文档](https://go.dev/doc/)
-   [SQLite WAL](https://www.sqlite.org/wal.html)

------------------------------------------------------------------------

## 最终建议

先把 AgentPulse
当作一个**可靠的本地采集与分析产品**，而不是先做一个漂亮的实时大屏。

优先顺序是：

1.  证明 Codex 能稳定输出所需遥测。
2.  建立可测试的 OTLP 接收和规范化事件模型。
3.  加入 Windows 进程采集与可信的时间区间关联。
4.  实现能解释数据来源的时间线与 Token 分析。
5.  最后完善 UI、安装包和其他 Agent 的适配接口。

只要第一版在"实时采集、调用链分析、命令耗时和数据质量"上可靠，后续适配自研
Harness 和其他 Agent 就主要是增加数据源 adapter，而不需要重写分析引擎。
