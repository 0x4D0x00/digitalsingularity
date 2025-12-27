package interceptor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	datahandle "digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/silicoid/mcp"
)

// 规范化的工具类型常量（遵循 tools schema）
const (
	ToolTypeFunctionCall = "function_call"
	ToolTypeToolCall     = "tool_call"
)

// ServerCall 表示一个服务端调用（通用命名，可能是 MCP 或其他类型）
type ServerCall struct {
	Type      string                 `json:"type"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	ID        string                 `json:"id"` // tool_call_id，用于匹配工具调用结果
}

// filterServerCallsInResponse 过滤响应中的服务端调用并确保 message 有 id
func (s *SilicoIDInterceptor) filterServerCallsInResponse(response map[string]interface{}, messages []interface{}) map[string]interface{} {
	// 提取 choices 数组
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return response
	}

	// 从 messages 历史中找到最后一条 assistant 消息的 id（如果存在）
	var lastAssistantID string
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "assistant" {
			if id, ok := msg["id"].(string); ok && id != "" {
				lastAssistantID = id
				break
			}
		}
	}

	// 遍历每个 choice
	for i, choiceInterface := range choices {
		choice, ok := choiceInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// 获取 message
		message, ok := choice["message"].(map[string]interface{})
		if !ok {
			continue
		}

		// 确保 message 有 id
		if _, hasID := message["id"]; !hasID {
			// 如果 messages 历史中有最后一条 assistant 消息的 id，使用它
			if lastAssistantID != "" {
				message["id"] = lastAssistantID
			} else {
				// 否则生成一个新的 id
				message["id"] = uuid.New().String()
			}
		}

		// 获取 content
		content, ok := message["content"].(string)
		if !ok || content == "" {
			continue
		}

		// If the response already contains structured tool call fields (tool_calls / function_call),
		// remove any embedded JSON codeblocks that duplicate those calls to avoid duplication
		// on the frontend (which may parse both).
		structuredCallsPresent := false
		if _, hasTopToolCalls := response["tool_calls"]; hasTopToolCalls {
			structuredCallsPresent = true
		} else if choicesRaw, ok2 := response["choices"].([]interface{}); ok2 && len(choicesRaw) > 0 {
			// check for function_call in choices
			for _, ch := range choicesRaw {
				if chMap, ok3 := ch.(map[string]interface{}); ok3 {
					if msg, ok4 := chMap["message"].(map[string]interface{}); ok4 {
						if _, hasFC := msg["function_call"]; hasFC {
							structuredCallsPresent = true
							break
						}
					}
				}
			}
		}

		cleanContent := content
		if structuredCallsPresent {
			cleanContent = removeEmbeddedToolJsonBlocks(cleanContent)
		}

		// 过滤 content 中的服务端调用（例如 THINK 标签）
		filteredContent := s.filterServerCalls(cleanContent)

		// 更新 content
		message["content"] = filteredContent
		choice["message"] = message
		choices[i] = choice
	}

	// 更新 response
	response["choices"] = choices
	return response
}

// convertExecutorResultToString 将执行结果转换为字符串
func (s *SilicoIDInterceptor) convertExecutorResultToString(result interface{}) string {
	switch v := result.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		// 对于其他类型，序列化为 JSON 字符串
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			logger.Printf("⚠️ 无法序列化 执行 结果: %v", err)
			return fmt.Sprintf("%v", result)
		}
		return string(jsonBytes)
	}
}

// removeTags 移除所有 [THINK] 标签（不包含执行调用的）
// 因为思考内容已经通过 chat_think 消息类型发送给前端，所以需要从最终响应中完全移除
func removeTags(content string) string {
	result := content

	// 移除 [THINK]...[/THINK] 标签（不包含执行调用的）
	reTHINK := regexp.MustCompile("(?s)\\[THINK\\](.*?)\\[/THINK\\]")
	result = reTHINK.ReplaceAllStringFunc(result, func(match string) string {
		// 无条件移除 THINK 标签中的内容——思考内容应通过独立的 chat_think 消息下发
		return ""
	})

	return result
}

// filterServerCalls 现在仅做最小化处理：移除标签并返回原文
// （服务器当前不执行任何工具调用；该函数保留以便未来扩展）
func (s *SilicoIDInterceptor) filterServerCalls(content string) string {
	// 只保留标签清理，其他解析/执行逻辑已移除
	return removeTags(content)
}

// removeEmbeddedToolJsonBlocks 移除 content 中与工具调用重复的 JSON 代码块
// 当响应中已经包含结构化的 tool_calls / function_call 时，应避免在 content
// 中同时携带同样的调用信息，前端会重复检测并导致重复执行/显示。
func removeEmbeddedToolJsonBlocks(content string) string {
	// 匹配 ```json ... ``` 代码块，dotAll 模式以匹配多行
	reCodeBlock := regexp.MustCompile("(?s)```json(.*?)```")

	// 先统计将要移除的代码块数量（用于关键日志）
	matches := reCodeBlock.FindAllStringSubmatch(content, -1)
	removedCount := 0
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		inner := m[1]
		if strings.Contains(inner, "\"function\"") ||
			strings.Contains(inner, "\"function_call\"") ||
			strings.Contains(inner, "\"tool_calls\"") ||
			strings.Contains(inner, "\"tool_call\"") {
			removedCount++
		}
	}
	if removedCount > 0 {
		logger.Printf("⚠️ 移除嵌入的 tool JSON 代码块: %d 个", removedCount)
	}

	// 执行实际替换（保留非工具相关的 codeblock）
	return reCodeBlock.ReplaceAllStringFunc(content, func(match string) string {
		submatches := reCodeBlock.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		inner := submatches[1]
		if strings.Contains(inner, "\"function\"") ||
			strings.Contains(inner, "\"function_call\"") ||
			strings.Contains(inner, "\"tool_calls\"") ||
			strings.Contains(inner, "\"tool_call\"") {
			return ""
		}
		return match
	})
}

// detectServerCalls 从 AI 响应中检测服务端调用（占位）
func (s *SilicoIDInterceptor) detectServerCalls(content string, requestID string) []ServerCall {
	// 服务器端当前不主动解析/执行内嵌工具调用——保留占位，未来可恢复
	return nil
}

// extractStructuredCallsFromResponse 尝试从模型响应的结构化字段中提取工具调用（优先使用）
// 支持 OpenAI 的 function_call 以及可能的 tool_calls 字段（包括由 formatconverter 转换后的结构）
func (s *SilicoIDInterceptor) extractStructuredCallsFromResponse(response map[string]interface{}, requestID string) []ServerCall {
	logger.Printf("[%s] extractStructuredCallsFromResponse 被调用", requestID)
	// 严格按照 tools schema 提取结构化调用（支持 top-level tool_calls 与 choices[].message.function_call）
	var calls []ServerCall

	// 优先处理 top-level tool_calls（标准 schema）
	if topToolCalls, ok := response["tool_calls"].([]interface{}); ok && len(topToolCalls) > 0 {
		for _, item := range topToolCalls {
			if itemMap, ok := item.(map[string]interface{}); ok {
				name, _ := itemMap["name"].(string)
				if name == "" {
					continue
				}
				args := map[string]interface{}{}
				if argObj, ok := itemMap["arguments"].(map[string]interface{}); ok {
					args = argObj
				} else if argStr, ok := itemMap["arguments"].(string); ok && argStr != "" {
					_ = json.Unmarshal([]byte(argStr), &args)
				}
				// 根据工具名称前缀判断执行类型
				// mcp_ 前缀的工具由服务器执行
				// client_ 前缀的工具由客户端执行
				// 其他工具根据数据库配置判断
				isServerExecutor := false
				if strings.HasPrefix(name, "mcp_") {
					isServerExecutor = true
					logger.Printf("[%s] 工具 %s 因 mcp_ 前缀被识别为服务器执行", requestID, name)
				} else if !strings.HasPrefix(name, "client_") {
					// 只有非 client_ 前缀的工具才检查数据库配置
					if execType, ok := ToolsExecutionType[name]; ok && execType == "server_executor" {
						isServerExecutor = true
						logger.Printf("[%s] 工具 %s 根据数据库配置被识别为服务器执行 (type=%s)", requestID, name, execType)
					} else {
						logger.Printf("[%s] 工具 %s 不是服务器执行工具 (ToolsExecutionType size=%d, has_key=%v, execType=%v)",
							requestID, name, len(ToolsExecutionType), ToolsExecutionType[name] != "", execType)
					}
				} else {
					logger.Printf("[%s] 工具 %s 因 client_ 前缀被识别为客户端执行", requestID, name)
				}

				if isServerExecutor {
					calls = append(calls, ServerCall{
						Type:      ToolTypeToolCall,
						Name:      name,
						Arguments: args,
					})
				}
			}
		}
		if len(calls) > 0 {
			logger.Printf("[%s] 返回 %d 个服务器端工具调用", requestID, len(calls))
			return calls
		}
	}

	// 其次处理 OpenAI 的 function_call 和 tool_calls（choices[0].message）
	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		if firstChoice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := firstChoice["message"].(map[string]interface{}); ok {
				// 处理 choices[0].message.tool_calls
				if msgToolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(msgToolCalls) > 0 {
					for _, item := range msgToolCalls {
						if itemMap, ok := item.(map[string]interface{}); ok {
							// 从 tool_calls 项中提取 function 信息
							if funcInfo, ok := itemMap["function"].(map[string]interface{}); ok {
								name, _ := funcInfo["name"].(string)
								if name == "" {
									continue
								}
								// 获取tool_call_id
								toolCallID, _ := itemMap["id"].(string)

								args := map[string]interface{}{}
								if argObj, ok := funcInfo["arguments"].(map[string]interface{}); ok {
									args = argObj
								} else if argStr, ok := funcInfo["arguments"].(string); ok && argStr != "" {
									_ = json.Unmarshal([]byte(argStr), &args)
								}
								// 根据工具名称前缀判断执行类型
								isServerExecutor := false
								if strings.HasPrefix(name, "mcp_") {
									isServerExecutor = true
									logger.Printf("[%s] 工具 %s 因 mcp_ 前缀被识别为服务器执行", requestID, name)
								} else if !strings.HasPrefix(name, "client_") {
									// 只有非 client_ 前缀的工具才检查数据库配置
									if execType, ok := ToolsExecutionType[name]; ok && execType == "server_executor" {
										isServerExecutor = true
										logger.Printf("[%s] 工具 %s 根据数据库配置被识别为服务器执行 (type=%s)", requestID, name, execType)
									} else {
										logger.Printf("[%s] 工具 %s 不是服务器执行工具 (ToolsExecutionType size=%d, has_key=%v, execType=%v)",
											requestID, name, len(ToolsExecutionType), ToolsExecutionType[name] != "", execType)
									}
								} else {
									logger.Printf("[%s] 工具 %s 因 client_ 前缀被识别为客户端执行", requestID, name)
								}

								if isServerExecutor {
									calls = append(calls, ServerCall{
										Type:      ToolTypeToolCall,
										Name:      name,
										Arguments: args,
										ID:        toolCallID,
									})
									logger.Printf("[%s] 添加服务器调用: name=%s, id=%s", requestID, name, toolCallID)
								}
							}
						}
					}
				}

				// 处理 choices[0].message.function_call（向后兼容）
				if fcRaw, ok := msg["function_call"].(map[string]interface{}); ok {
					name, _ := fcRaw["name"].(string)
					if name != "" {
						args := map[string]interface{}{}
						if argObj, ok := fcRaw["arguments"].(map[string]interface{}); ok {
							args = argObj
						} else if argStr, ok := fcRaw["arguments"].(string); ok && argStr != "" {
							_ = json.Unmarshal([]byte(argStr), &args)
						}
						// 为旧的function_call格式生成ID
						functionCallID := fmt.Sprintf("%s:function_call", name)

						// 根据工具名称前缀判断执行类型
						isServerExecutor := false
						if strings.HasPrefix(name, "mcp_") {
							isServerExecutor = true
							logger.Printf("[%s] 工具 %s (function_call) 因 mcp_ 前缀被识别为服务器执行", requestID, name)
						} else if !strings.HasPrefix(name, "client_") {
							// 只有非 client_ 前缀的工具才检查数据库配置
							if execType, ok := ToolsExecutionType[name]; ok && execType == "server_executor" {
								isServerExecutor = true
								logger.Printf("[%s] 工具 %s (function_call) 根据数据库配置被识别为服务器执行 (type=%s)", requestID, name, execType)
							} else {
								logger.Printf("[%s] 工具 %s (function_call) 不是服务器执行工具", requestID, name)
							}
						} else {
							logger.Printf("[%s] 工具 %s (function_call) 因 client_ 前缀被识别为客户端执行", requestID, name)
						}

						if isServerExecutor {
							calls = append(calls, ServerCall{
								Type:      ToolTypeFunctionCall,
								Name:      name,
								Arguments: args,
								ID:        functionCallID,
							})
							logger.Printf("[%s] 添加服务器调用 (function_call): name=%s, id=%s", requestID, name, functionCallID)
						}
					}
				}
			}
		}
	}
	return calls
}

// parseServerCall 解析单个服务端调用（占位）
func (s *SilicoIDInterceptor) parseServerCall(jsonContent string, requestID string) *ServerCall {
	// 彻底简化：不再解析复杂的内嵌调用，保留占位以便未来实现
	return nil
}

// loadMCPServerConfigs 加载MCP服务器配置
func (s *SilicoIDInterceptor) loadMCPServerConfigs() ([]mcp.MCPServerConfig, error) {
	// 从格式转换器中复用配置加载逻辑
	if s.formatConverter != nil {
		// 由于格式转换器的方法是私有的，我们直接读取配置文件
		return s.loadMCPServerConfigsFromFile()
	}

	return s.loadMCPServerConfigsFromFile()
}

// getStringValue 从interface{}中安全获取字符串值
func getStringValue(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}
// loadMCPServerConfigsFromFile 从文件加载MCP服务器配置
func (s *SilicoIDInterceptor) loadMCPServerConfigsFromFile() ([]mcp.MCPServerConfig, error) {
	file, err := os.Open("backend/silicoid/mcp.json")
	if err != nil {
		return nil, fmt.Errorf("无法打开MCP配置文件: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取MCP配置文件失败: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析MCP配置文件失败: %v", err)
	}

	// 提取mcpServers数组
	mcpServersRaw, ok := config["mcpServers"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("MCP配置文件中没有mcpServers字段")
	}

	var mcpServers []mcp.MCPServerConfig
	for _, serverRaw := range mcpServersRaw {
		serverMap, ok := serverRaw.(map[string]interface{})
		if !ok {
			continue
		}

		config := mcp.MCPServerConfig{
			Type:               getStringValue(serverMap["type"]),
			URL:                getStringValue(serverMap["url"]),
			Name:               getStringValue(serverMap["name"]),
			Description:        getStringValue(serverMap["description"]),
			AuthorizationToken: getStringValue(serverMap["authorization_token"]),
		}

		mcpServers = append(mcpServers, config)
	}

	return mcpServers, nil
}
// appendServerCallResultToMessages 将执行结果追加到消息列表
func (s *SilicoIDInterceptor) appendServerCallResultToMessages(messages []interface{}, calls []ServerCall, results []string) []interface{} {
	// 构建结果消息
	var resultText strings.Builder
	resultText.WriteString("工具调用结果：\n\n")

	for i, call := range calls {
		resultText.WriteString(fmt.Sprintf("### %s\n", call.Name))
		if i < len(results) {
			resultText.WriteString(results[i])
		} else {
			resultText.WriteString("执行失败")
		}
		resultText.WriteString("\n\n")
	}

	// 添加用户消息
	messages = append(messages, map[string]interface{}{
		"id":      uuid.New().String(),
		"role":    "user",
		"content": resultText.String(),
	})

	return messages
}

// ========== 公共执行处理方法（可供 WebSocket 和 HTTP 使用） ==========

// extractContent 从文本中提取 `[THINK]` 标签的内容（通用命名，便于未来支持其他标签）
// 返回提取的标签内容（多个标签内容合并，用换行分隔）
func extractContent(text string) string {
	var contents []string

	// 提取 [THINK]...[/THINK] 标签内容
	reTHINK := regexp.MustCompile("(?s)\\[THINK\\](.*?)\\[/THINK\\]")
	matches := reTHINK.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			content := strings.TrimSpace(match[1])
			if content != "" {
				contents = append(contents, content)
			}
		}
	}

	// 合并所有提取到的内容
	if len(contents) > 0 {
		return strings.Join(contents, "\n")
	}

	return ""
}

// ExtractThinkContent 从文本中提取 THINK 内容（公开方法，供外部调用）
func (s *SilicoIDInterceptor) ExtractThinkContent(text string) string {
	return extractContent(text)
}

// RemoveThinkTags 移除内容中的 [THINK]...[/THINK] 标签
func (s *SilicoIDInterceptor) RemoveThinkTags(content string) string {
	re := regexp.MustCompile(`(?s)\\[THINK\\].*?\\[/THINK\\]`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}

// SaveToolCallContext 保存工具调用上下文到Redis
func (s *SilicoIDInterceptor) SaveToolCallContext(userID string, requestID string, requestData map[string]interface{}, aiResponse map[string]interface{}) error {
	svc, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		return fmt.Errorf("创建数据库服务失败: %v", err)
	}

	currentModelName, _ := requestData["model"].(string)
	currentRoleName, _ := requestData["role_name"].(string)

	storeObj := map[string]interface{}{
		"initiator":   userID,
		"model":       currentModelName,
		"role_name":   currentRoleName,
		"user_id":     userID,
		"request_id":  requestID,
		"detected_at": time.Now().Unix(),
	}

	if msgs, ok := requestData["messages"].([]interface{}); ok {
		storeObj["messages_snapshot"] = msgs
	}

	if aiResponse != nil {
		storeObj["ai_response"] = aiResponse
	}

	key := fmt.Sprintf("tools_call_context:%s", requestID)
	if res := svc.RedisWrite(key, storeObj, 5*time.Minute); res == nil || res.Status != datahandle.StatusSuccess {
		return fmt.Errorf("保存client_executor_call上下文到Redis失败")
	}

	logger.Printf("[%s] 已保存client_executor_call上下文到Redis (key=%s)", requestID, key)
	return nil
}

// LoadToolCallContext 从Redis加载工具调用上下文
func (s *SilicoIDInterceptor) LoadToolCallContext(sessionID string) (map[string]interface{}, error) {
	svc, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		return nil, fmt.Errorf("创建数据库服务失败: %v", err)
	}

	if res := svc.RedisRead(fmt.Sprintf("tools_call_context:%s", sessionID)); res.IsSuccess() {
		var storedCtx map[string]interface{}
		switch v := res.Data.(type) {
		case map[string]interface{}:
			storedCtx = v
		case string:
			_ = json.Unmarshal([]byte(v), &storedCtx)
		}
		return storedCtx, nil
	}

	return nil, fmt.Errorf("未找到会话上下文")
}

// UpdateToolCallContext 更新工具调用上下文
func (s *SilicoIDInterceptor) UpdateToolCallContext(sessionID string, messages []interface{}, aiResponse map[string]interface{}) error {
	svc, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		return fmt.Errorf("创建数据库服务失败: %v", err)
	}

	updateCtx := map[string]interface{}{
		"messages_snapshot": messages,
		"ai_response":       aiResponse,
	}

	key := fmt.Sprintf("tools_call_context:%s", sessionID)
	if res := svc.RedisWrite(key, updateCtx, 5*time.Minute); res == nil || res.Status != datahandle.StatusSuccess {
		return fmt.Errorf("更新会话上下文到Redis失败")
	}

	logger.Printf("[%s] 已更新会话上下文到Redis (key=%s)", sessionID, key)
	return nil
}

// ExtractServerCall 从 AI 响应中提取第一个服务端调用
// 返回: (call, prefixText, hasCall)
// prefixText 包含调用之前的文本，以及从标签中提取的思考内容
// initiator: 可选，表示是谁发起的对话（用于在检测到 tools schema 格式时写入 Redis）
// requestID: 可选，用于生成 Redis key 以便后续查找
func (s *SilicoIDInterceptor) ExtractServerCall(response string, initiator string, requestID string) (*ServerCall, string, bool) {
	response = strings.TrimSpace(response)
	// 严格遵循 tools schema：只接受标准的 code-block JSON（```json ... ```）或 function_call 结构
	// 不再尝试把整个回复当任意 JSON 解析（不向后兼容旧宽松格式）
	start := 0

	for {
		// 查找下一个 ```json 标记
		jsonStart := strings.Index(response[start:], "```json")
		if jsonStart == -1 {
			break
		}
		jsonStart += start

		// 查找 JSON 代码块的结束标记
		searchStart := jsonStart + 7 // 跳过 ```json
		jsonEnd := strings.Index(response[searchStart:], "```")
		if jsonEnd == -1 {
			break
		}
		jsonEnd = searchStart + jsonEnd

		// 提取 JSON 内容
		jsonContent := response[jsonStart+7 : jsonEnd]
		jsonContent = strings.TrimSpace(jsonContent)

		// 尝试解析为 ServerCall（必须包含 name）
		var serverCallBlock ServerCall
		if err := json.Unmarshal([]byte(jsonContent), &serverCallBlock); err == nil {
			if serverCallBlock.Name != "" {
				// 找到了调用，提取之前的文本作为 prefixText
				prefixText := strings.TrimSpace(response[:jsonStart])
				contentExtracted := extractContent(prefixText)
				if contentExtracted != "" {
					prefixText = contentExtracted
				}
				// 若未指定 Type，则默认视为 function_call
				if serverCallBlock.Type == "" {
					serverCallBlock.Type = ToolTypeFunctionCall
				}
				// 仅接受标准类型
				if serverCallBlock.Type == ToolTypeFunctionCall || serverCallBlock.Type == ToolTypeToolCall {
					// 如果提供了 initiator 和 requestID，则将检测到的调用上下文写入 Redis，方便 client executor 或结果处理读取
					if initiator != "" && requestID != "" {
						go func(call ServerCall, initiatorVal, reqID string) {
							svc, err := datahandle.NewCommonReadWriteService("database")
							if err != nil {
								logger.Printf("[%s] ⚠️ 无法初始化 CommonReadWriteService: %v", reqID, err)
								return
							}
							storeObj := map[string]interface{}{
								"initiator":   initiatorVal,
								"call_name":   call.Name,
								"call_type":   call.Type,
								"arguments":   call.Arguments,
								"detected_at": time.Now().Unix(),
								"request_id":  reqID,
							}
							key := fmt.Sprintf("tools_call_context:%s", reqID)
							res := svc.RedisWrite(key, storeObj, 5*time.Minute)
							if res == nil || res.Status != datahandle.StatusSuccess {
								var errDetail interface{}
								if res != nil {
									errDetail = res.Error
								} else {
									errDetail = "nil result"
								}
								logger.Printf("[%s] ⚠️ 保存 tools_call_context 到 Redis 失败: %v", reqID, errDetail)
							} else {
								logger.Printf("[%s] ✅ 已将 tools 调用元数据存储到 Redis key=%s", reqID, key)
							}
						}(serverCallBlock, initiator, requestID)
					}
					return &serverCallBlock, prefixText, true
				}
			}
		}

		// 继续查找下一个代码块
		start = jsonEnd + 3
	}

	// 作为严格的非-codeblock 备选，检测 "function_call" 关键词并尝试提取其 JSON 对象
	if idx := strings.Index(response, `"function_call"`); idx >= 0 {
		// 在关键词后找第一个 '{' 并尝试匹配到对应 '}'（简化匹配）
		openIdx := strings.Index(response[idx:], "{")
		if openIdx >= 0 {
			openIdx += idx
			closeIdx := strings.Index(response[openIdx:], "}")
			if closeIdx > 0 {
				closeIdx += openIdx
				jsonPart := response[openIdx : closeIdx+1]
				var fcObj map[string]interface{}
				if err := json.Unmarshal([]byte(jsonPart), &fcObj); err == nil {
					name, _ := fcObj["name"].(string)
					if name != "" {
						args := map[string]interface{}{}
						if argObj, ok := fcObj["arguments"].(map[string]interface{}); ok {
							args = argObj
						} else if argStr, ok := fcObj["arguments"].(string); ok && argStr != "" {
							_ = json.Unmarshal([]byte(argStr), &args)
						}
						serverCall := &ServerCall{
							Type:      ToolTypeFunctionCall,
							Name:      name,
							Arguments: args,
						}
						thinkContent := extractContent(response[:idx])
						// 保存检测到的调用上下文到 Redis（如果提供了 requestID 和 initiator）
						if requestID != "" && initiator != "" {
							go func(call *ServerCall, initiatorVal, reqID string) {
								svc, err := datahandle.NewCommonReadWriteService("database")
								if err != nil {
									logger.Printf("[%s] ⚠️ 无法初始化 CommonReadWriteService: %v", reqID, err)
									return
								}
								storeObj := map[string]interface{}{
									"initiator":   initiatorVal,
									"call_name":   call.Name,
									"call_type":   call.Type,
									"arguments":   call.Arguments,
									"detected_at": time.Now().Unix(),
									"request_id":  reqID,
								}
								key := fmt.Sprintf("tools_call_context:%s", reqID)
								res := svc.RedisWrite(key, storeObj, 5*time.Minute)
								if res == nil || res.Status != datahandle.StatusSuccess {
									var errDetail interface{}
									if res != nil {
										errDetail = res.Error
									} else {
										errDetail = "nil result"
									}
									logger.Printf("[%s] ⚠️ 保存 tools_call_context 到 Redis 失败: %v", reqID, errDetail)
								} else {
									logger.Printf("[%s] ✅ 已将 tools 调用元数据存储到 Redis key=%s", reqID, key)
								}
							}(serverCall, initiator, requestID)
						}
						return serverCall, thinkContent, true
					}
				}
			}
		}
	}

	// 未检测到标准的工具调用
	return nil, "", false
}



type TagCallback func(content string) error

// ProcessClientExecutorResult 处理前端回传的 client_executor_result
// 这个方法负责处理工具执行结果的后处理，包括会话上下文管理和消息链重构
func (s *SilicoIDInterceptor) ProcessClientExecutorResult(
	ctx context.Context,
	requestID string,
	userID string,
	messageData map[string]interface{},
	sendMessage func(messageType string, data map[string]interface{}) error,
) error {
	logger.Printf("[%s] 处理 client_executor_result (user=%s)", requestID, userID)

	// 提取原始调用和结果
	originalCall := messageData["original_call_json"]
	result := messageData["result"]
	contextMap, _ := messageData["context"].(map[string]interface{})

	// 构造用户消息，将前端返回的结果投喂给 AI
	var resultStr string
	if result != nil {
		if b, err := json.Marshal(result); err == nil {
			resultStr = string(b)
		} else {
			resultStr = fmt.Sprintf("%v", result)
		}
	}

	userMessage := fmt.Sprintf("工具调用的执行结果：\n原始调用：%s\n\n执行结果：%s", originalCall, resultStr)

	// 构建请求数据，尽量从回传的 context 中恢复必要字段（model/role_name/messages快照）
	requestData := map[string]interface{}{}
	// 优先使用直接回传的 contextMap 字段
	if contextMap != nil {
		if m, ok := contextMap["model"].(string); ok && m != "" {
			requestData["model"] = m
		}
		if rn, ok := contextMap["role_name"].(string); ok && rn != "" {
			requestData["role_name"] = rn
		}
	}

	// 确保 requestData 中包含 user_id
	if userID != "" {
		requestData["user_id"] = userID
	}

	// 从Redis恢复工具调用上下文
	origRequestID := ""
	if contextMap != nil {
		logger.Printf("[%s] contextMap不为空，包含键: %v", requestID, getMapKeys(contextMap))
		if sid, ok := contextMap["session_id"].(string); ok && sid != "" {
			origRequestID = sid
			logger.Printf("[%s] 从contextMap获取到origRequestID: %s", requestID, origRequestID)
		} else {
			logger.Printf("[%s] contextMap中没有找到有效的session_id", requestID)
		}
	} else {
		logger.Printf("[%s] contextMap为空", requestID)
	}

	// 如果没有从contextMap获取到origRequestID，尝试从requestID推断
	// client_executor_result的requestID格式是 CER_{user}_{timestamp}
	// 对应的原始requestID格式是 CHAT_{user}_{timestamp}
	if origRequestID == "" && strings.HasPrefix(requestID, "CER_") {
		// 从 CER_yXO0tZpxhgW_1766557355574539999 提取 CHAT_yXO0tZpxhgW_1766557351923761205
		parts := strings.Split(requestID, "_")
		if len(parts) >= 3 {
			origRequestID = "CHAT_" + parts[1] + "_" + parts[2]
			logger.Printf("[%s] 从requestID推断出origRequestID: %s", requestID, origRequestID)
		}
	}

	// 定义 storedCtx 变量，用于后续的消息链构建
	var storedCtx map[string]interface{}
	if origRequestID != "" {
		var err error
		storedCtx, err = s.LoadToolCallContext(origRequestID)
		if err != nil {
			logger.Printf("[%s] 加载会话上下文失败: %v", requestID, err)
		} else if storedCtx != nil {
			logger.Printf("[%s] 成功从Redis加载上下文，包含键: %v", requestID, getMapKeys(storedCtx))
			if m, ok := storedCtx["model"].(string); ok && m != "" {
				requestData["model"] = m
			}
			if rn, ok := storedCtx["role_name"].(string); ok && rn != "" {
				requestData["role_name"] = rn
			}
			if initiator, ok := storedCtx["initiator"].(string); ok && initiator != "" {
				requestData["user_id"] = initiator
			}
			if snap, ok := storedCtx["messages_snapshot"].([]interface{}); ok && len(snap) > 0 {
				requestData["messages"] = snap
				logger.Printf("[%s] 从Redis恢复了messages_snapshot，共 %d 条消息", requestID, len(snap))
			}
			// 从Redis恢复时也需要包含ai_response（包含tool_calls的assistant消息）
			if aiResp, ok := storedCtx["ai_response"].(map[string]interface{}); ok {
				requestData["ai_response"] = aiResp
				logger.Printf("[%s] 从Redis恢复了ai_response，消息内容: role=%v, has_tool_calls=%v, has_function_call=%v",
					aiResp["role"], aiResp["tool_calls"] != nil, aiResp["function_call"] != nil)
			}
		} else {
			logger.Printf("[%s] 从Redis加载的上下文为空", requestID)
		}
	}

	// 如果没有提供 model，使用默认值
	if _, ok := requestData["model"]; !ok {
		requestData["model"] = "deepseek-chat"
	}

	// 恢复 messages_snapshot 并构造完整的消息链
	var messages []interface{}

	// 优先使用从Redis恢复的上下文
	if storedCtx != nil {
		if snap, ok := storedCtx["messages_snapshot"].([]interface{}); ok && len(snap) > 0 {
			messages = append(messages, snap...)
			logger.Printf("[%s] 从Redis恢复了 %d 条历史消息", requestID, len(snap))
		}
		if aiResp, ok := storedCtx["ai_response"].(map[string]interface{}); ok {
			// 确保assistant消息包含tool_calls
			if toolCalls, hasToolCalls := aiResp["tool_calls"]; hasToolCalls {
				messages = append(messages, aiResp)
				logger.Printf("[%s] 从Redis恢复了包含tool_calls的assistant消息，tool_calls数量: %d", requestID, len(toolCalls.([]interface{})))
			} else if _, hasFunctionCall := aiResp["function_call"]; hasFunctionCall {
				messages = append(messages, aiResp)
				logger.Printf("[%s] 从Redis恢复了包含function_call的assistant消息", requestID)
			} else {
				logger.Printf("[%s] ⚠️ 从Redis恢复的assistant消息不包含工具调用，消息内容: %+v", requestID, aiResp)
			}
		}
	} else {
		// 回退到直接从requestData恢复
		if msgs, ok := requestData["messages"].([]interface{}); ok && len(msgs) > 0 {
			messages = append(messages, msgs...)
		}
		if aiResp, ok := requestData["ai_response"].(map[string]interface{}); ok {
			// 确保assistant消息包含tool_calls
			if toolCalls, hasToolCalls := aiResp["tool_calls"]; hasToolCalls {
				messages = append(messages, aiResp)
				logger.Printf("[%s] 从requestData恢复了包含tool_calls的assistant消息，tool_calls数量: %d", requestID, len(toolCalls.([]interface{})))
			} else if _, hasFunctionCall := aiResp["function_call"]; hasFunctionCall {
				messages = append(messages, aiResp)
				logger.Printf("[%s] 从requestData恢复了包含function_call的assistant消息", requestID)
			} else {
				logger.Printf("[%s] ⚠️ 从requestData恢复的assistant消息不包含工具调用，消息内容: %+v", requestID, aiResp)
			}
		}
	}

	// 添加工具执行结果作为 tool 消息
	var toolCallId string
	if origCall, ok := messageData["original_call_json"].(map[string]interface{}); ok {
		if id, ok := origCall["id"].(string); ok {
			toolCallId = id
		}
	}

	toolMessage := map[string]interface{}{
		"role":         "tool",
		"tool_call_id": toolCallId,
		"content":      userMessage,
	}
	messages = append(messages, toolMessage)
	requestData["messages"] = messages

	// 调试：输出完整的消息链
	logger.Printf("[%s] 构建完成的消息链，共 %d 条消息:", requestID, len(messages))
	for i, msg := range messages {
		if msgMap, ok := msg.(map[string]interface{}); ok {
			role, _ := msgMap["role"].(string)
			content := ""
			if c, ok := msgMap["content"].(string); ok && len(c) > 50 {
				content = c[:50] + "..."
			} else if c, ok := msgMap["content"].(string); ok {
				content = c
			}
			if role == "assistant" {
				if _, hasToolCalls := msgMap["tool_calls"]; hasToolCalls {
					logger.Printf("[%s]   消息 %d: role=%s, has_tool_calls=true, content=%s", requestID, i, role, content)
				} else if _, hasFunctionCall := msgMap["function_call"]; hasFunctionCall {
					logger.Printf("[%s]   消息 %d: role=%s, has_function_call=true, content=%s", requestID, i, role, content)
				} else {
					logger.Printf("[%s]   消息 %d: role=%s, content=%s", requestID, i, role, content)
				}
			} else {
				logger.Printf("[%s]   消息 %d: role=%s, content=%s", requestID, i, role, content)
			}
		}
	}

	// 重新调用AI
	resp, err := s.CreateWebSocketNonStreamResponse(ctx, requestData, requestID)
	if err != nil {
		logger.Printf("[%s] 重新调用AI失败: %v", requestID, err)
		return fmt.Errorf("重新调用AI失败: %v", err)
	}

	// 检查AI响应是否包含新的工具调用
	hasNewToolCalls := false
	if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if _, ok := message["tool_calls"]; ok {
					hasNewToolCalls = true
				} else if _, ok := message["function_call"]; ok {
					hasNewToolCalls = true
				}
			}
		}
	}

	if hasNewToolCalls {
		// AI又返回了工具调用，发送新的client_executor_call
		logger.Printf("[%s] AI基于工具结果又返回了新的工具调用", requestID)

		var newCalls []interface{}
		var newAssistantMessage map[string]interface{}
		if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if toolCalls, ok := message["tool_calls"]; ok {
						if tcArr, ok := toolCalls.([]interface{}); ok {
							newCalls = tcArr
						}
					} else if fc, ok := message["function_call"]; ok {
						newCalls = []interface{}{fc}
					}

					newAssistantMessage = map[string]interface{}{
						"role":    "assistant",
						"content": "",
					}
					if content, ok := message["content"].(string); ok {
						newAssistantMessage["content"] = content
					}
					if toolCalls, ok := message["tool_calls"]; ok {
						newAssistantMessage["tool_calls"] = toolCalls
					}
					if fc, ok := message["function_call"]; ok {
						newAssistantMessage["function_call"] = fc
					}
				}
			}
		}

		if len(newCalls) > 0 {
			// 保存新的assistant消息到Redis
			if newAssistantMessage != nil {
				go func() {
					err := s.UpdateToolCallContext(origRequestID, messages, newAssistantMessage)
					if err != nil {
						logger.Printf("[%s] 更新会话上下文失败: %v", requestID, err)
					}
				}()
			}

			clientCallData := map[string]interface{}{
				"type":  "client_executor_call",
				"calls": newCalls,
			}

			if origRequestID != "" {
				clientCallData["session_id"] = origRequestID
			}

			return sendMessage("client_executor_call", clientCallData)
		}
	} else {
		// AI返回了最终回答
		logger.Printf("[%s] AI基于工具结果返回了最终回答", requestID)

		finalContent := ""
		if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						finalContent = content
					}
				}
			}
		}

		// 分离THINK内容和最终回答
		thinkContent := s.ExtractThinkContent(finalContent)
		if thinkContent != "" {
			thinkData := map[string]interface{}{
				"type":      "chat_think",
				"content":   thinkContent,
				"text":      thinkContent,
				"timestamp": time.Now().Unix(),
			}
			if err := sendMessage("chat_think", thinkData); err != nil {
				logger.Printf("[%s] 发送THINK内容失败: %v", requestID, err)
			}
		}

		// 发送清理后的最终回答
		cleanContent := s.RemoveThinkTags(finalContent)
		finalData := map[string]interface{}{
			"type":      "chat_complete",
			"content":   cleanContent,
			"timestamp": time.Now().Unix(),
		}

		if origRequestID != "" {
			finalData["session_id"] = origRequestID
		}

		return sendMessage("chat_complete", finalData)
	}

	return nil
}

// getMapKeys 获取map的所有键名，用于调试
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
