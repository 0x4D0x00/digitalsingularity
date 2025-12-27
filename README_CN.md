# Silicoid Core — PotAGI 后端 · 0x4D

Silicoid Core 是 PotAGI 后端 — Silicoid AI 的大脑模块，由 0x4D 设计和维护。

联系方式：moycox@Outlook.com · +86 18667048877 · 中国杭州

## 项目概述

Silicoid Core 是一个面向"模型集成与能力编排"的后端平台。设计用于灵活嵌入各类大语言模型与多模态能力（文本/图像/文档/语音），同时将提示词、角色、工具与路由配置放在后端数据库中统一管理，降低上游系统与前端的复杂度。

## 核心能力

- **多模型适配**：动态加载/缓存模型配置并按规则路由请求（支持云端与自托管模型）
- **格式转换层**：OpenAI ↔ Claude 等格式双向转换，自动处理文件/图片上传与降级策略
- **全模态输入处理**：原生支持 `image`/`document`/`pdf`/base64 上传、文件分块与智能文本提取
- **工具执行框架**：支持 `tools`/`tool_calls`/`function_call` 格式，区分 `client_executor` 与 `server_executor`，便于插件化能力接入与工具驱动的 agent 流程
- **会话与上下文管理**：Redis 存储对话上下文与工具调用上下文，支持客户端执行器回调与多步编排
- **安全与密钥策略**：并行支持平台 API key 与用户自有 key，内置 API key 管理、余额限制与自动失效策略
- **部署与运维友好**：HTTP/WebSocket/MCP 等服务可按需独立启动，内置日志、端口管理与部署示例

## 设计亮点

- **数据库驱动的提示词与配置**：系统提示词、角色模板、工具定义与模型路由均存储于数据库，后端负责注入与版本管理，前端无需拼装复杂提示词
- **中央格式转换层**：`formatconverter` 把多模态/多厂商的请求/响应规范化为统一格式，简化上层调用
- **可插拔模型管理**：`ModelManager` 提供缓存+DB 双层配置、API key 池与优先级选择，易于接入新的模型提供商或自托管服务
- **MCP 与能力网格**：支持通过配置接入外部能力节点（MCP），形成能力网格与集群化扩展

## 关键代码路径（验证证据）

- **服务编排与启动**：`main.go`（启动参数：`-httpOnly`、`-websocketOnly`、`-silicoidHttpOnly` 等）
- **模型管理**：`backend/silicoid/models/manager/service.go`（`ModelManager`、`GetModelConfig`）
- **格式转换与文件处理**：`backend/silicoid/formatconverter/*`（`NormalizeOpenAIRequest`、`processContentArray`、Claude Files API 支持）
- **请求预处理与多模态上传**：`backend/silicoid/interceptor/service.go`（`processFilesInRequest`、`authenticateAndPreprocessRequest`、`ProcessClientExecutorResult`）
- **工具与执行器注入**：`backend/silicoid/formatconverter/service.go`（`AddExecutorTools`）、`extractStructuredCallsFromResponse`（拦截器）
- **配置/提示词数据结构**：`backend/data_structure/aibasicplatform/`（系统提示词 / 角色 / 工具 SQL 结构）

## 快速开始（最小示例）

1. **环境**：安装 Go（推荐 1.20+）并准备数据库与 Redis
2. **构建**：
   ```bash
   go build -o silicoid-core ./...
   ```
3. **运行**（示例：仅启动 HTTP 服务）：
   ```bash
   ./silicoid-core -httpOnly -httpPort 20717 -logLevel INFO
   ```
4. **验证点**：
   - 通过 `POST` 请求访问 SilicoID/OpenAI 兼容接口（如 `/silicoid/models`、`/silicoid`），观察服务返回与日志
   - 尝试带 `role_name` 或 `file_read` 参数的请求，观察服务器端如何读取数据库提示词、上传文件并转换（查看日志中的 `file_id`、上传与降级记录）

## 扩展性与实用优势

- **运行时接入任意模型后端**：通过 `ModelManager` 与可配置的 `model_code`/`base_url` 实现无缝接入与灰度切换
- **插件化工具与能力**：工具定义可在数据库中扩展，按 `client_executor`/`server_executor` 分发执行，方便把第三方服务、前端插件或内部微服务当作能力节点嵌入
- **多模态兼容与降级策略**：优先使用云厂商文件 API（如 Claude Files），失败时自动降级为 base64 嵌入或文本摘要，保证兼容性与鲁棒性
- **后端统一管理提示词与角色**：减少前端复杂度、支持版本回滚与 A/B 测试、便于跨项目复用与审计
- **运维与成本控制**：API key 池、优先级选择与熔断机制支持按成本/质量路由请求

## 许可与使用

本仓库采用永久 Business Source License (BSL)。

- **商业/生产用途**：须事先获得作者（0x4D）的书面授权并签订商业许可协议
- **非商业/学术/评估用途**：允许使用，但须通过邮件通知作者以便备案（发送至 `moycox@Outlook.com`，主题请写明"Silicoid Core 非商业使用通知"并简要说明用途）
- **商业授权流程**：请通过 email `moycox@Outlook.com` 联系，后续以邮件沟通报价与合同细节

仓库包含 `LICENSE`（BSL 简明摘要 + 完整文本）、`CONTRIBUTING.md` 与 `CLA.md` 以明确贡献与商业权利归属。

## 贡献与协作

欢迎社区贡献（遵循 `CONTRIBUTING.md` 并签署 `CLA`）。贡献将用于改进平台与示例，但商业授权权利由作者保留。

欢迎提交 issue、bug 报告与增强建议；请优先提交带复现步骤的 issue。

## 作者与联系方式

- **作者**：0x4D
- **邮箱**：moycox@Outlook.com
- **电话**：+86 18667048877
- **所在地**：中国杭州
