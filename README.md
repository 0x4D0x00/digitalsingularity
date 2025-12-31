# Silicoid Core — PotAGI backend · 0x4D

[![License: BSL 1.1](https://img.shields.io/badge/License-BSL%201.1-blue.svg)](https://github.com/0x4D0x00/digitalsingularity/blob/main/LICENSE)

Silicoid Core is the PotAGI backend — the brain module for the Silicoid AI, designed and maintained by 0x4D.

Contact: moycox@Outlook.com · +86 18667048877 · Hangzhou, China

## Overview

Silicoid Core is a backend platform designed for "model integration and capability orchestration". It's built to flexibly embed various large language models and multimodal capabilities (text/image/document/speech), while managing prompts, roles, tools, and routing configurations in the backend database to reduce complexity for upstream systems and frontends.

## Core Capabilities

- **Multi-Model Integration**: Dynamic loading and caching of model configurations with intelligent routing (supports both cloud and self-hosted models)
- **Format Conversion Layer**: Bidirectional conversion between OpenAI ↔ Claude formats, with automatic file/image upload handling and fallback strategies
- **Multimodal Input Processing**: Native support for `image`/`document`/`pdf`/base64 uploads, file chunking, and intelligent text extraction
- **Tool Execution Framework**: Support for `tools`/`tool_calls`/`function_call` formats, distinguishing between `client_executor` and `server_executor` for pluggable capability integration and tool-driven agent workflows
- **Session & Context Management**: Redis-based conversation and tool call context storage, supporting client executor callbacks and multi-step orchestration
- **Security & Key Management**: Parallel support for platform API keys and user-owned keys, with built-in API key management, balance limiting, and automatic failure handling
- **Deployment & Operations**: HTTP/WebSocket/MCP services can be started independently, with built-in logging, port management, and deployment examples

## Design Highlights

- **Database-Driven Prompts & Configuration**: System prompts, role templates, tool definitions, and model routing are all stored in the backend database, with the backend handling injection and version management — no complex prompt assembly needed in the frontend
- **Central Format Conversion Layer**: The `formatconverter` normalizes multimodal/multi-vendor requests and responses into unified formats, simplifying upstream calls
- **Pluggable Model Management**: `ModelManager` provides dual-layer (cache + DB) configuration, API key pooling, and priority selection for easy integration of new model providers or self-hosted services
- **MCP & Capability Mesh**: Supports configuration-driven integration of external capability nodes (MCP) to form capability meshes and cluster extensions

## Key Code Paths (Verification Evidence)

- **Service Orchestration & Startup**: `main.go` (startup flags: `-httpOnly`, `-websocketOnly`, `-silicoidHttpOnly`, etc.)
- **Model Management**: `backend/silicoid/models/manager/service.go` (`ModelManager`, `GetModelConfig`)
- **Format Conversion & File Processing**: `backend/silicoid/formatconverter/*` (`NormalizeOpenAIRequest`, `processContentArray`, Claude Files API support)
- **Request Preprocessing & Multimodal Uploads**: `backend/silicoid/interceptor/service.go` (`processFilesInRequest`, `authenticateAndPreprocessRequest`, `ProcessClientExecutorResult`)
- **Tool & Executor Injection**: `backend/silicoid/formatconverter/service.go` (`AddExecutorTools`), `extractStructuredCallsFromResponse` (interceptor)
- **Configuration/Prompt Data Structure**: `backend/data_structure/aibasicplatform/` (system prompt / roles / tools SQL schemas)

## Quick Start (Minimal Example)

1. **Environment**: Install Go (1.20+ recommended) and prepare database + Redis
2. **Build**:
   ```bash
   go build -o silicoid-core ./...
   ```
3. **Run** (example: HTTP service only):
   ```bash
   ./silicoid-core -httpOnly -httpPort 20717 -logLevel INFO
   ```
4. **Verification Points**:
   - Make `POST` requests to SilicoID/OpenAI-compatible endpoints (e.g., `/silicoid/models`, `/silicoid`) and observe service responses and logs
   - Try requests with `role_name` or `file_read` parameters and observe how the server reads database prompts, uploads files, and performs conversions (check logs for `file_id`, upload, and fallback records)

## Extensibility & Practical Advantages

- **Runtime Integration of Any Model Backend**: Seamless integration and canary deployments via `ModelManager` with configurable `model_code`/`base_url`
- **Pluggable Tools & Capabilities**: Tool definitions extendable in database, distributed as `client_executor`/`server_executor`, enabling embedding of third-party services, frontend plugins, or internal microservices as capability nodes
- **Multimodal Compatibility & Fallback Strategies**: Prioritizes cloud vendor file APIs (Claude Files), with automatic fallback to base64 embedding or text summaries for robustness
- **Backend-Centralized Prompt & Role Management**: Reduces frontend complexity, enables version rollback and A/B testing, facilitates cross-project reuse and auditing
- **Operations & Cost Control**: API key pooling, priority selection, and circuit breaker mechanisms enable cost/quality-based request routing

## License & Usage

This repository uses a permanent Business Source License (BSL).

- **Commercial/Production Use**: Requires explicit written authorization and commercial license agreement from the author
- **Non-Commercial/Academic/Evaluation Use**: Permitted but requires email notification to the author for record-keeping (send to `moycox@Outlook.com` with subject "Silicoid Core non-commercial use notification" and brief description of intended use)
- **Commercial Licensing Process**: Contact via email at `moycox@Outlook.com` for pricing and contract details

### Commercial Licensing

For organizations interested in commercial licensing of Silicoid Core:

1. **Contact**: moycox@Outlook.com
2. **Subject**: "Silicoid Core Commercial License Inquiry"
3. **Provide**: Your organization details, intended use case, and expected usage scale
4. **Response**: You'll receive licensing terms, pricing, and contract details

Commercial licenses are available for production deployment, redistribution, and commercial AI service development.

## Support & Sponsorship

### Commercial Licensing
For commercial use of Silicoid Core, please contact:
- **Email**: moycox@Outlook.com
- **Subject**: "Silicoid Core Commercial License Inquiry"
- **Phone**: +86 18667048877

### Community Support
- **GitHub Issues**: Report bugs and suggestions
- **Pull Requests**: Contribute code improvements
- **Discussions**: Technical exchanges and sharing

### Sponsorship (Optional)
If you'd like to support project development, consider:
- 💰 **Afdian**: [https://ifdian.net/a/0x4D0x00](https://ifdian.net/a/0x4D0x00) - WeChat/Alipay support
- 📧 **Business Partnerships**: Contact for collaboration opportunities

The repository includes `LICENSE` (BSL summary + full text), `CONTRIBUTING.md`, and `CLA.md` to clarify contribution and commercial rights.

## Contributing & Collaboration

Community contributions are welcome (follow `CONTRIBUTING.md` and sign the `CLA`). Contributions improve the platform and examples while preserving commercial licensing rights for the author.

Issues, bug reports, and enhancement suggestions are encouraged — please include reproduction steps where possible.

## Author & Contact

- **Author**: 0x4D
- **Email**: moycox@Outlook.com
- **Phone**: +86 18667048877
- **Location**: Hangzhou, China  