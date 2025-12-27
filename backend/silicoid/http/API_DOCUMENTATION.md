# SilicoID HTTP API 接口文档

## 概述

SilicoID HTTP 服务提供 OpenAI 兼容的 API 接口，用于与各种 AI 模型进行交互。服务默认运行在端口 `30717`，支持 CORS 跨域请求。

**服务地址：** `http://127.0.0.1:30717`

**重要说明：**
- 所有接口都支持 CORS 跨域请求
- 所有接口都支持 OPTIONS 预检请求
- 响应格式为 JSON

---

## 基础接口

### 1. 服务根路径

**接口地址：** `GET /`

**说明：** 服务根路径，用于检查服务是否正常运行。

**请求参数：** 无

**响应示例：**
```
SilicoID HTTP 服务
```

---

## OpenAI 兼容接口

### 2. 聊天完成

**接口地址：** `POST /v1/chat/completions`

**说明：** OpenAI 兼容的聊天完成接口，支持流式和非流式响应。

**认证方式：**
- **方式1：** 在请求头中提供 `Authorization: Bearer <api_key>` 或 `Authorization: Bearer <token>`
- **方式2：** 在请求体中提供 `token` 字段

**API Key 类型说明：**
- `sk-potagi-xxx`：平台认证 Key，需要验证用户身份和余额，会扣费
- `sk-ant-xxx`：用户自己的 Claude Key，不验证不扣费，直接使用
- `sk-xxx`（其他格式）：用户自己的 OpenAI Key，不验证不扣费，直接使用
- 其他格式：作为平台认证 Key 验证

**请求格式：**
```json
{
  "model": "deepseek-chat",
  "messages": [
    {
      "role": "system",
      "content": "你是一个有用的助手。"
    },
    {
      "role": "user",
      "content": "你好，请介绍一下你自己。"
    }
  ],
  "stream": true,
  "temperature": 0.7,
  "max_tokens": 1000
}
```

**请求字段说明：**

**必需字段：**
- `model` (string, 必需): 模型名称，必须是真实的模型版本名称
  - 示例：`deepseek-chat`、`gpt-4o`、`gpt-4o-mini`、`gpt-3.5-turbo`、`claude-3-7-sonnet-20250219`、`claude-3-opus-20240229`、`llama-3.1-8b-instruct` 等
  - 可通过 `/v1/models` 接口获取所有可用的模型列表
- `messages` (array, 必需): 消息数组，详见下方 messages 字段说明

**可选字段：**
- `stream` (boolean, 可选): 是否使用流式响应，默认为 `false`
- `temperature` (number, 可选): 采样温度，范围 0-2，默认 1.0
- `max_tokens` (integer, 可选): 最大生成 token 数
- `top_p` (number, 可选): 核采样参数，范围 0-1
- `frequency_penalty` (number, 可选): 频率惩罚，范围 -2.0 到 2.0
- `presence_penalty` (number, 可选): 存在惩罚，范围 -2.0 到 2.0
- `stop` (string | array, 可选): 停止序列
- `token` (string, 可选): 用户认证 token（如果未在请求头中提供）

**messages 字段说明：**

`messages` 是一个消息数组，用于构建对话历史。每个消息对象包含以下字段：

**必需字段：**
- `role` (string, 必需): 消息的角色，必须是以下值之一：
  - `"system"`: 系统消息，用于设置助手的行为和角色
  - `"user"`: 用户消息，表示用户的输入
  - `"assistant"`: 助手消息，表示模型的回复
  - `"function"`: 函数消息，用于函数调用的结果（如果支持函数调用）
- `content` (string | object, 必需): 消息内容
  - 字符串格式：直接文本内容
  - 对象格式（多模态，如果模型支持）：
    ```json
    {
      "type": "text",
      "text": "文本内容"
    }
    ```
    或
    ```json
    {
      "type": "image_url",
      "image_url": {
        "url": "图片URL或base64编码"
      }
    }
    ```

**可选字段：**
- `name` (string, 可选): 参与者的名称
- `tool_calls` (array, 可选): 工具调用列表
- `tool_call_id` (string, 可选): 工具调用的ID

**流式响应格式：**

当 `stream: true` 时，响应为 Server-Sent Events (SSE) 格式：

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234567890,"model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant","content":"你好"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234567890,"model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"！"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234567890,"model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

**非流式响应格式：**

当 `stream: false` 或未设置时，响应为 JSON 格式：

```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "deepseek-chat",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "你好！我是一个AI助手..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30
  }
}
```

**错误响应格式：**

```json
{
  "error": {
    "message": "错误描述",
    "type": "invalid_request_error",
    "code": "invalid_api_key"
  }
}
```

**常见错误码：**
- `400 Bad Request`: 请求格式错误或缺少必要参数
- `401 Unauthorized`: 认证失败（API Key 或 Token 无效）
- `402 Payment Required`: 令牌余额不足（仅在使用平台 Key 时）
- `500 Internal Server Error`: 服务器内部错误

**使用示例：**

**JavaScript 示例（流式）：**
```javascript
const response = await fetch('http://127.0.0.1:30717/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer sk-potagi-xxx'
  },
  body: JSON.stringify({
    model: 'deepseek-chat',
    messages: [
      { role: 'user', content: '你好' }
    ],
    stream: true
  })
});

const reader = response.body.getReader();
const decoder = new TextDecoder();

while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  
  const chunk = decoder.decode(value);
  const lines = chunk.split('\n');
  
  for (const line of lines) {
    if (line.startsWith('data: ')) {
      const data = line.slice(6);
      if (data === '[DONE]') break;
      
      const json = JSON.parse(data);
      if (json.choices[0].delta.content) {
        console.log(json.choices[0].delta.content);
      }
    }
  }
}
```

**JavaScript 示例（非流式）：**
```javascript
const response = await fetch('http://127.0.0.1:30717/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer sk-potagi-xxx'
  },
  body: JSON.stringify({
    model: 'deepseek-chat',
    messages: [
      { role: 'user', content: '你好' }
    ],
    stream: false
  })
});

const result = await response.json();
console.log(result.choices[0].message.content);
```

---

### 3. 获取模型列表

**接口地址：** `GET /v1/models`

**说明：** 获取所有可用的模型列表。此接口是公开接口，不需要认证。

**请求参数：** 无

**响应格式：**
```json
{
  "object": "list",
  "data": [
    {
      "id": "deepseek-chat",
      "object": "model",
      "created": 1234567890,
      "owned_by": "DeepSeek"
    },
    {
      "id": "gpt-4o",
      "object": "model",
      "created": 1234567890,
      "owned_by": "GPT"
    },
    {
      "id": "claude-3-7-sonnet-20250219",
      "object": "model",
      "created": 1234567890,
      "owned_by": "Claude"
    },
    {
      "id": "llama-3.1-8b-instruct",
      "object": "model",
      "created": 1234567890,
      "owned_by": "Llama"
    }
  ]
}
```

**响应字段说明：**
- `object` (string, 必需): 固定为 "list"，表示这是一个列表响应
- `data` (array, 必需): 模型对象数组，包含所有可用的模型

**模型对象字段说明：**
- `id` (string, 必需): 模型名称（真实版本名称），用于在聊天接口的 `model` 字段中使用
  - 示例：`deepseek-chat`、`gpt-4o`、`claude-3-7-sonnet-20250219`、`llama-3.1-8b-instruct` 等
  - **重要：** 必须使用此字段的值作为聊天接口中的 `model` 参数，而不是提供商名称
- `object` (string, 必需): 固定为 "model"
- `created` (integer, 必需): 模型创建时间戳（Unix 时间戳，秒级）
- `owned_by` (string, 必需): 模型提供商名称
  - 示例：`DeepSeek`、`GPT`、`Claude`、`Llama`、`Anthropic`、`OpenAI` 等

**使用示例：**

```javascript
const response = await fetch('http://127.0.0.1:30717/v1/models', {
  method: 'GET',
  headers: {
    'Content-Type': 'application/json'
  }
});

const result = await response.json();
console.log('可用模型列表:', result.data);

// 使用模型 ID
const modelId = result.data[0].id; // 例如: "deepseek-chat"
```

**重要提示：**
1. **模型 ID 使用：** 在聊天接口中使用 `model` 字段时，必须使用此接口返回的 `id` 字段值（如 `deepseek-chat`、`gpt-4o` 等），而不是提供商名称（如 `DeepSeek`、`GPT` 等）
2. **模型列表来源：** 模型列表从数据库的各公司模型表中动态加载，如果加载失败会降级到已配置的模型列表
3. **无需认证：** 此接口是公开接口，不需要提供用户 token 或 API 密钥
4. **CORS 支持：** 此接口支持跨域请求（CORS）

---

## 模型维护接口

### 4. 同步所有提供商的模型列表

**接口地址：** `POST /v1/models/sync/all`

**说明：** 同步所有提供商的模型列表到数据库。此接口会从各个 AI 提供商的 API 获取最新的模型列表并更新到数据库。

**请求参数：** 无

**响应格式：**
```json
{
  "success": true,
  "message": "成功同步所有提供商的模型列表"
}
```

**错误响应格式：**
```json
{
  "error": true,
  "message": "同步模型失败: 错误描述"
}
```

**使用示例：**

```javascript
const response = await fetch('http://127.0.0.1:30717/v1/models/sync/all', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json'
  }
});

const result = await response.json();
if (result.success) {
  console.log('同步成功:', result.message);
} else {
  console.error('同步失败:', result.message);
}
```

**重要提示：**
- 此接口会调用各个提供商的 API 获取模型列表，可能需要较长时间
- 同步过程中会使用数据库中已配置的 baseURL 和 API Key
- 如果某个提供商的同步失败，不会影响其他提供商的同步

---

### 5. 同步指定提供商的模型列表

**接口地址：** `POST /v1/models/sync/{provider}`

**说明：** 同步指定提供商的模型列表到数据库。可以从请求体中提供临时的 baseURL 和 API Key，也可以使用数据库中已配置的配置。

**路径参数：**
- `provider` (string, 必需): 提供商名称，如 `DeepSeek`、`GPT`、`Claude`、`Anthropic`、`OpenAI`、`Llama` 等

**请求体（可选）：**
```json
{
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx"
}
```

**请求字段说明：**
- `base_url` (string, 可选): API 基础 URL，如果不提供则使用数据库中的配置
- `api_key` (string, 可选): API 密钥，如果不提供则使用数据库中的配置

**响应格式：**
```json
{
  "success": true,
  "provider": "DeepSeek",
  "message": "成功同步 DeepSeek 的模型列表"
}
```

**错误响应格式：**
```json
{
  "error": true,
  "message": "同步模型失败: 错误描述"
}
```

**使用示例：**

**使用数据库配置：**
```javascript
const response = await fetch('http://127.0.0.1:30717/v1/models/sync/DeepSeek', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json'
  }
});

const result = await response.json();
if (result.success) {
  console.log('同步成功:', result.message);
} else {
  console.error('同步失败:', result.message);
}
```

**使用临时配置：**
```javascript
const response = await fetch('http://127.0.0.1:30717/v1/models/sync/DeepSeek', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    base_url: 'https://api.deepseek.com/v1',
    api_key: 'sk-xxx'
  })
});

const result = await response.json();
if (result.success) {
  console.log('同步成功:', result.message);
} else {
  console.error('同步失败:', result.message);
}
```

**重要提示：**
1. **提供商名称：** 必须使用正确的提供商名称，如 `DeepSeek`、`GPT`、`Claude`、`Anthropic`、`OpenAI`、`Llama` 等
2. **配置优先级：** 如果提供了 `base_url` 和 `api_key`，则使用提供的配置；否则使用数据库中的配置
3. **同步范围：** 只同步指定提供商的模型列表，不会影响其他提供商

---

## 错误处理

所有接口在出错时都会返回以下格式：

```json
{
  "error": {
    "message": "错误描述",
    "type": "错误类型",
    "code": "错误代码"
  }
}
```

或（模型维护接口）：

```json
{
  "error": true,
  "message": "错误描述"
}
```

**常见错误码：**
- `400 Bad Request`: 请求格式错误或缺少必要参数
- `401 Unauthorized`: 认证失败（API Key 或 Token 无效）
- `402 Payment Required`: 令牌余额不足（仅在使用平台 Key 时）
- `500 Internal Server Error`: 服务器内部错误

---

## CORS 支持

所有接口都支持 CORS 跨域请求，配置如下：

- **Allowed Origins:** `*`（允许所有来源）
- **Allowed Methods:** `GET`, `POST`, `PUT`, `DELETE`, `OPTIONS`
- **Allowed Headers:** `Content-Type`, `Authorization`

---

## 注意事项

1. **模型名称：** 在聊天接口中使用 `model` 字段时，必须使用 `/v1/models` 接口返回的 `id` 字段值，而不是提供商名称
2. **认证方式：** 聊天接口支持多种认证方式，包括平台 API Key、用户自己的 API Key 和 Token
3. **流式响应：** 聊天接口支持流式和非流式响应，通过 `stream` 参数控制
4. **模型同步：** 模型维护接口用于同步模型列表，通常由管理员调用
5. **服务端口：** 默认服务端口为 `30717`，可通过启动参数修改

---

## 完整使用示例

### 1. 获取模型列表

```javascript
// 获取所有可用模型
const modelsResponse = await fetch('http://127.0.0.1:30717/v1/models');
const modelsData = await modelsResponse.json();
console.log('可用模型:', modelsData.data);
```

### 2. 发送聊天请求（非流式）

```javascript
// 发送聊天请求
const chatResponse = await fetch('http://127.0.0.1:30717/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer sk-potagi-xxx'
  },
  body: JSON.stringify({
    model: 'deepseek-chat',
    messages: [
      { role: 'user', content: '你好' }
    ],
    stream: false
  })
});

const chatData = await chatResponse.json();
console.log('回复:', chatData.choices[0].message.content);
```

### 3. 发送聊天请求（流式）

```javascript
// 发送流式聊天请求
const chatResponse = await fetch('http://127.0.0.1:30717/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer sk-potagi-xxx'
  },
  body: JSON.stringify({
    model: 'deepseek-chat',
    messages: [
      { role: 'user', content: '你好' }
    ],
    stream: true
  })
});

const reader = chatResponse.body.getReader();
const decoder = new TextDecoder();
let fullContent = '';

while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  
  const chunk = decoder.decode(value);
  const lines = chunk.split('\n');
  
  for (const line of lines) {
    if (line.startsWith('data: ')) {
      const data = line.slice(6);
      if (data === '[DONE]') break;
      
      try {
        const json = JSON.parse(data);
        if (json.choices && json.choices[0].delta.content) {
          const content = json.choices[0].delta.content;
          fullContent += content;
          process.stdout.write(content); // 实时输出
        }
      } catch (e) {
        // 忽略解析错误
      }
    }
  }
}

console.log('\n完整回复:', fullContent);
```

### 4. 同步模型列表

```javascript
// 同步所有提供商的模型列表
const syncResponse = await fetch('http://127.0.0.1:30717/v1/models/sync/all', {
  method: 'POST'
});

const syncData = await syncResponse.json();
if (syncData.success) {
  console.log('同步成功');
} else {
  console.error('同步失败:', syncData.message);
}

**curl 示例（同步所有提供商的模型列表）：**

```bash
curl -X POST "http://127.0.0.1:30717/v1/models/sync/all" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-potagi-xxx" \
  -d '{}' 
```
```

---

## 版本信息

- **文档版本：** 1.0
- **最后更新：** 2025-01-XX
- **服务版本：** 根据实际部署版本

