package interceptor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"digitalsingularity/backend/silicoid/mcp"
)

// ExecuteServerCall 执行单个服务端调用（统一入口）
// 支持 server_executor 类型的服务端工具
func (s *SilicoIDInterceptor) ExecuteServerCall(ctx context.Context, call *ServerCall, requestID string) (string, error) {
	// 根据工具的execution_type决定如何执行
	execType, ok := ToolsExecutionType[call.Name]
	if !ok {
		return "", fmt.Errorf("未知工具类型: %s", call.Name)
	}

	switch execType {
	case "server_executor":
		callTypeLabel := "服务端执行器"
		logger.Printf("[%s] 🚀 执行 %s 调用: %s (type: %s)", requestID, callTypeLabel, call.Name, call.Type)

		// 检查是否为MCP工具
		if strings.HasPrefix(call.Name, "mcp_") {
			return s.executeMCPTool(ctx, *call, requestID)
		}

		// 其他服务器端工具调用（目前不支持）
		logger.Printf("[%s] ⚠️ ExecuteServerCall: unsupported server tool, call=%s", requestID, call.Name)
		return "", fmt.Errorf("unsupported server tool: %s", call.Name)
	default:
		return "", fmt.Errorf("不支持的执行类型: %s", execType)
	}
}
// executeServerCall 执行单个服务端调用
func (s *SilicoIDInterceptor) executeServerCall(ctx context.Context, call ServerCall, requestID string) (string, error) {
	logger.Printf("[%s] 执行服务端工具调用: %s", requestID, call.Name)

	// 根据工具名称判断是否为MCP工具
	if strings.HasPrefix(call.Name, "mcp_") {
		return s.executeMCPTool(ctx, call, requestID)
	}

	// 其他服务器端工具调用（目前不支持）
	logger.Printf("[%s] ⚠️ executeServerCall: unsupported server tool, call=%s", requestID, call.Name)
	return "", fmt.Errorf("unsupported server tool: %s", call.Name)
}
// executeMCPTool 执行MCP工具调用
func (s *SilicoIDInterceptor) executeMCPTool(ctx context.Context, call ServerCall, requestID string) (string, error) {
	// 从工具名称中提取服务器名称
	// 格式: mcp_{server_name}_{tool_name}
	parts := strings.SplitN(call.Name, "_", 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid MCP tool name format: %s", call.Name)
	}

	serverName := parts[1]
	// 使用完整的工具名称（包括mcp_前缀），因为MCP服务器现在直接处理完整名称
	toolName := call.Name

	logger.Printf("[%s] 执行MCP工具: server=%s, tool=%s", requestID, serverName, toolName)

	// 加载MCP服务器配置
	serverConfigs, err := s.loadMCPServerConfigs()
	if err != nil {
		return "", fmt.Errorf("加载MCP服务器配置失败: %v", err)
	}

	// 查找对应的服务器配置
	var serverConfig *mcp.MCPServerConfig
	for _, config := range serverConfigs {
		if config.Name == serverName {
			serverConfig = &config
			break
		}
	}

	if serverConfig == nil {
		return "", fmt.Errorf("未找到MCP服务器配置: %s", serverName)
	}

	// 获取或创建MCP客户端
	client := s.mcpClientManager.GetClient(serverName, serverConfig)

	// 执行工具调用
	mcpToolCall := &mcp.MCPToolCall{
		Name:      toolName,
		Arguments: call.Arguments,
	}

	result, err := client.CallTool(ctx, mcpToolCall)
	if err != nil {
		logger.Printf("[%s] MCP工具调用失败: %v", requestID, err)
		return "", fmt.Errorf("MCP工具调用失败: %v", err)
	}

	logger.Printf("[%s] MCP工具调用成功: %s", requestID, toolName)
	return fmt.Sprintf("%v", result), nil
}
// ProcessAIResponseWithStructuredServerCalls 处理包含结构化服务器调用的AI响应
// 这个方法接收已经提取的服务器调用列表，直接执行这些调用
func (s *SilicoIDInterceptor) ProcessAIResponseWithStructuredServerCalls(
	ctx context.Context,
	requestData map[string]interface{},
	serverCalls []ServerCall,
	response map[string]interface{}, // 完整的AI响应，包含choices[0].message等
	isClaudeModel bool,
	requestID string,
	maxIterations int,
	enableTTS bool,
	voiceGender string,
	ttsCallback SynthesisResultCallback,
	tagCallback TagCallback,
) (string, error) {
	logger.Printf("[%s] 🔍 开始处理结构化服务器调用，调用数量: %d", requestID, len(serverCalls))

	if len(serverCalls) == 0 {
		// 如果没有服务器调用，直接从响应中提取内容并返回
		if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						logger.Printf("[%s] ✅ 没有服务器调用，直接返回AI内容", requestID)
						return s.filterServerCalls(content), nil
					}
				}
			}
		}
		return "", fmt.Errorf("无法从响应中提取内容")
	}

	// 获取当前的 messages
	messages, ok := requestData["messages"].([]interface{})
	if !ok {
		logger.Printf("[%s] ❌ 无效的 messages 格式", requestID)
		return "", fmt.Errorf("无效的 messages 格式")
	}

	// 保存原始的 role_name
	originalRoleName, _ := requestData["role_name"].(string)
	logger.Printf("[%s] 📌 保存原始 role_name: %s", requestID, originalRoleName)

	// 执行服务器调用
	logger.Printf("[%s] 🔧 开始执行 %d 个服务器调用", requestID, len(serverCalls))
	results := make([]string, len(serverCalls))

	for i, call := range serverCalls {
		logger.Printf("[%s] 执行调用 %d: %s", requestID, i+1, call.Name)
		result, err := s.ExecuteServerCall(ctx, &call, requestID)
		if err != nil {
			logger.Printf("[%s] ❌ 服务器调用 %s 执行失败: %v", requestID, call.Name, err)
			results[i] = fmt.Sprintf("执行失败: %v", err)
		} else {
			logger.Printf("[%s] ✅ 服务器调用 %s 执行成功", requestID, call.Name)
			results[i] = result
		}
	}

	// 将服务器调用结果添加到消息链中
	// 首先添加AI的assistant消息（保留tool_calls，因为tool消息需要对应的tool_call）
	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				// 构建完整的assistant消息，保留tool_calls
				assistantMsg := map[string]interface{}{
					"role":    "assistant",
					"content": "",
				}
				if content, ok := message["content"].(string); ok {
					assistantMsg["content"] = content
				}
				// 保留tool_calls，因为tool消息需要有对应的tool_call_id引用
				if toolCalls, ok := message["tool_calls"]; ok {
					assistantMsg["tool_calls"] = toolCalls
				}
				if functionCall, ok := message["function_call"]; ok {
					assistantMsg["function_call"] = functionCall
				}

				messages = append(messages, assistantMsg)
				logger.Printf("[%s] 已添加assistant消息到消息链（保留tool_calls）", requestID)
			}
		}
	}

	// 添加工具执行结果
	for i, call := range serverCalls {
		toolCallID := call.ID
		if toolCallID == "" {
			// 向后兼容：如果没有ID，生成一个
			toolCallID = fmt.Sprintf("%s:%d", call.Name, i)
		}

		toolMsg := map[string]interface{}{
			"role":         "tool",
			"tool_call_id": toolCallID,
			"content":      fmt.Sprintf("工具调用结果：\n\n### %s\n%s", call.Name, results[i]),
		}
		messages = append(messages, toolMsg)
		logger.Printf("[%s] 已添加工具结果消息到消息链: %s (tool_call_id=%s)", requestID, call.Name, toolCallID)
	}

	// 更新requestData中的messages
	requestData["messages"] = messages

	// 清除tools参数，避免FormatConverter重复添加工具
	// 因为消息链中已经包含了工具执行结果，不需要再次提供工具定义
	delete(requestData, "tools")
	delete(requestData, "_mcp_servers")

	// 重新调用AI获取最终回答
	logger.Printf("[%s] 🔄 重新调用AI获取基于工具结果的最终回答", requestID)

	// 恢复原始role_name
	if originalRoleName != "" {
		requestData["role_name"] = originalRoleName
	}

	// 再次调用AI
	finalResponse, err := s.CreateWebSocketNonStreamResponse(ctx, requestData, requestID)
	if err != nil {
		logger.Printf("[%s] ❌ 重新调用AI失败: %v", requestID, err)
		return "", fmt.Errorf("重新调用AI失败: %v", err)
	}

	// 从最终响应中提取内容
	if choices, ok := finalResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					logger.Printf("[%s] ✅ 获取到最终回答，长度: %d", requestID, len(content))
					return s.filterServerCalls(content), nil
				}
			}
		}
	}

	logger.Printf("[%s] ❌ 无法从最终响应中提取内容", requestID)
	return "", fmt.Errorf("无法从最终响应中提取内容")
}

func (s *SilicoIDInterceptor) ProcessAIResponseWithServerCalls(
	ctx context.Context,
	requestData map[string]interface{},
	initialResponse string,
	isClaudeModel bool,
	requestID string,
	maxIterations int,
	enableTTS bool,
	voiceGender string,
	ttsCallback SynthesisResultCallback,
	tagCallback TagCallback,
) (string, error) {
	logger.Printf("[%s] 🔍 开始执行循环处理，响应长度: %d", requestID, len(initialResponse))

	currentResponse := initialResponse
	iteration := 0

	// 获取当前的 messages
	messages, ok := requestData["messages"].([]interface{})
	if !ok {
		logger.Printf("[%s] ❌ 无效的 messages 格式", requestID)
		return "", fmt.Errorf("无效的 messages 格式")
	}

	// 保存原始的 role_name，用于再次调用时保持系统提示词类型
	originalRoleName, _ := requestData["role_name"].(string)
	if originalRoleName == "" {
		// 如果 requestData 中没有，尝试从其他地方获取
		// 这通常发生在 formatConverter 已经删除了 role_name 的情况下
		logger.Printf("[%s] ⚠️  requestData 中未找到 role_name，可能已被 formatConverter 删除", requestID)
	}
	logger.Printf("[%s] 📌 保存原始 role_name: %s", requestID, originalRoleName)

	// system_prompt 处理已移至格式转换器中，此处不再处理

	// 在第一次迭代前，检查是否有大文件分块需要分批投喂
	if err := s.FeedLargeFileChunks(ctx, requestID, requestData); err != nil {
		logger.Printf("[%s] ⚠️  大文件分批投喂失败: %v，继续处理", requestID, err)
		// 不中断流程，继续处理
	}

	for iteration < maxIterations {
		// 检查当前响应是否包含执行调用
		logger.Printf("[%s] 🔍 第 %d 次迭代，检查执行调用...", requestID, iteration)
		// 尝试从 requestData 中提取 initiator（优先），否则从 messages 中获取最后一条 user 消息的 id 或内容片段
		initiator := ""
		if uid, ok := requestData["user_id"].(string); ok && uid != "" {
			initiator = uid
		} else {
			// 尝试从 messages 中找到最后一个 role=user 的消息并使用其 id 或前50字符作为 initiator
			for i := len(messages) - 1; i >= 0; i-- {
				if msg, ok := messages[i].(map[string]interface{}); ok {
					if role, _ := msg["role"].(string); role == "user" {
						if id, ok := msg["id"].(string); ok && id != "" {
							initiator = id
						} else if content, ok := msg["content"].(string); ok && content != "" {
							if len(content) > 50 {
								initiator = content[:50]
							} else {
								initiator = content
							}
						}
						break
					}
				}
			}
		}
		serverCall, prefixText, hasCall := s.ExtractServerCall(currentResponse, initiator, requestID)
		if !hasCall {
			// 没有执行调用，返回最终响应（过滤掉任何残留的标签）
			logger.Printf("[%s] ✅ 未检测到执行调用，开始过滤并返回最终响应", requestID)
			filteredResponse := s.filterServerCalls(currentResponse)
			logger.Printf("[%s] ✅ 过滤完成，最终响应长度: %d", requestID, len(filteredResponse))

			// 如果启用了 TTS，在返回前进行语音合成
			if enableTTS && filteredResponse != "" && ttsCallback != nil {
				go s.synthesizeSpeechAsync(filteredResponse, voiceGender, requestID, ttsCallback)
			}

			return filteredResponse, nil
		}

		// 记录 prefixText 的检测情况（思考内容）
		if prefixText != "" {
			logger.Printf("[%s] 📋 第 %d 次迭代检测到思考内容（执行调用之前的文本），长度: %d，前100字符: %s",
				requestID, iteration, len(prefixText), truncateString(prefixText, 100))
		} else {
			logger.Printf("[%s] ⚠️  第 %d 次迭代未检测到思考内容（执行调用之前没有文本内容）", requestID, iteration)
		}

		// 如果检测到思考内容，先发送给前端（通过 chat_think 消息类型）
		// 注意：第一次迭代（iteration == 0）的思考内容已经在调用方（silicoid.go）中发送，
		// 所以这里只在后续迭代中发送，避免重复
		if prefixText != "" && tagCallback != nil && iteration > 0 {
			logger.Printf("[%s] 📤 检测到思考内容（第 %d 次迭代），发送给前端，长度: %d", requestID, iteration, len(prefixText))
			if err := tagCallback(prefixText); err != nil {
				logger.Printf("[%s] ❌ 发送思考内容失败: %v", requestID, err)
				// 不中断流程，继续处理
			}
		} else if iteration > 0 {
			logger.Printf("[%s] ⚠️  第 %d 次迭代未发送思考内容（prefixText为空或tagCallback为nil）", requestID, iteration)
		}

		iteration++
		logger.Printf("[%s] 🔄 执行循环 %d/%d: 处理执行调用 %s", requestID, iteration, maxIterations, serverCall.Name)

		// 根据工具名称前缀判断执行类型
		// client_ 前缀的工具由客户端执行，mcp_ 前缀的工具由服务器执行
		isClientExecutor := strings.HasPrefix(serverCall.Name, "client_")
		if !isClientExecutor {
			// 检查数据库配置的执行类型
			if execType, ok := ToolsExecutionType[serverCall.Name]; ok && execType == "client_executor" {
				isClientExecutor = true
			}
		}

		if isClientExecutor {
			logger.Printf("[%s] ⚪ 工具 %s 标记为 client_executor，放行给客户端执行（不在服务器执行）", requestID, serverCall.Name)
			// 返回当前响应（保留代码块供前端执行），不执行服务器端调用
			filteredResponse := s.filterServerCalls(currentResponse)
			return filteredResponse, nil
		}

		// 执行服务端调用（当前为占位，执行会返回未实现错误）
		execResult, err := s.ExecuteServerCall(ctx, serverCall, requestID)
		if err != nil {
			logger.Printf("[%s] ❌ 执行调用失败: %v", requestID, err)
			// 将错误信息返回给 AI
			execResult = fmt.Sprintf("执行调用失败: %v", err)
		}

		// 检查是否需要分批处理
		execResultStr := s.convertExecutorResultToString(execResult)
		const maxContentSize = 100000 // 10万字符限制

		if isLargeContent(execResultStr, maxContentSize) {
			logger.Printf("[%s] 📦 检测到大内容 (%d 字符)，开始分批处理", requestID, len(execResultStr))

			// 分批处理大内容
			chunks := chunkContent(execResultStr, serverCall.Name, maxContentSize)
			logger.Printf("[%s] 📦 内容已分为 %d 个批次", requestID, len(chunks))

			// 分批投喂给AI
			for i, chunk := range chunks {
				logger.Printf("[%s] 📤 投喂第 %d/%d 批次内容 (长度: %d)", requestID, i+1, len(chunks), len(chunk.Content))

				// 构造分批消息
				var batchMessage string
				if len(chunks) == 1 {
					// 只有一个批次，正常处理
					batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果：\n\n```json\n%s\n```\n\n请基于以上结果继续回答用户的问题。",
						serverCall.Name, chunk.Content)
				} else {
					// 多个批次，添加批次信息
					if chunk.IsLast {
						batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果 (第 %d/%d 批次，最后一批)：\n\n```json\n%s\n```\n\n所有数据已投喂完毕，请基于以上所有结果继续回答用户的问题。",
							serverCall.Name, chunk.Index, chunk.Total, chunk.Content)
					} else {
						batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果 (第 %d/%d 批次)：\n\n```json\n%s\n```\n\n这是第 %d 批数据，请等待所有数据投喂完毕后再回答。",
							serverCall.Name, chunk.Index, chunk.Total, chunk.Content, chunk.Index)
					}
				}

				// 更新 messages：添加 AI 的响应（包含前缀文本）和工具结果
				newMessages := make([]interface{}, len(messages))
				copy(newMessages, messages)

				// 添加 AI 的响应（只包含前缀文本，不包含 ）
				if prefixText != "" && i == 0 {
					// 只在第一批次时添加前缀文本
					newMessages = append(newMessages, map[string]interface{}{
						"role":    "assistant",
						"content": prefixText,
					})
				}

				// 添加工具结果
				newMessages = append(newMessages, map[string]interface{}{
					"role":    "user",
					"content": batchMessage,
				})

				// 更新 requestData
				requestData["messages"] = newMessages

				// 恢复原始的 role_name，确保再次调用时保持系统提示词类型
				if originalRoleName != "" {
					requestData["role_name"] = originalRoleName
				}

				// 如果不是最后一批，让AI等待
				if !chunk.IsLast {
					logger.Printf("[%s] ⏳ 第 %d 批次已投喂，等待AI确认接收", requestID, i+1)

					// 调用AI确认接收
					response, err := s.CreateWebSocketNonStreamResponse(ctx, requestData, requestID)
					if err != nil {
						logger.Printf("[%s] ❌ 批次确认调用 AI 失败: %v", requestID, err)
						return "", fmt.Errorf("批次确认调用 AI 失败: %v", err)
					}

					// 从响应中提取文本内容
					var confirmResponse string
					if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]interface{}); ok {
							if message, ok := choice["message"].(map[string]interface{}); ok {
								if content, ok := message["content"].(string); ok {
									confirmResponse = content
								}
							}
						}
					}

					logger.Printf("[%s] ✅ 第 %d 批次确认响应: %s", requestID, i+1, truncateString(confirmResponse, 100))

					// 更新messages，添加AI的确认响应
					messages = append(messages, map[string]interface{}{
						"id":      uuid.New().String(),
						"role":    "assistant",
						"content": confirmResponse,
					})
				} else {
					// 最后一批，让AI基于所有数据回答
					logger.Printf("[%s] 📤 最后一批次已投喂，让AI基于所有数据回答", requestID)

					response, err := s.CreateWebSocketNonStreamResponse(ctx, requestData, requestID)
					if err != nil {
						logger.Printf("[%s] ❌ 最终调用 AI 失败: %v", requestID, err)
						return "", fmt.Errorf("最终调用 AI 失败: %v", err)
					}

					// 从响应中提取文本内容
					var finalResponse string
					if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]interface{}); ok {
							if message, ok := choice["message"].(map[string]interface{}); ok {
								if content, ok := message["content"].(string); ok {
									finalResponse = content
								}
							}
						}
					}

					if finalResponse == "" {
						logger.Printf("[%s] ❌ AI 返回了空响应", requestID)
						return "", fmt.Errorf("AI 返回了空响应")
					}

					logger.Printf("[%s] ✅ AI 最终响应成功，长度: %d", requestID, len(finalResponse))
					currentResponse = finalResponse
					messages = newMessages
				}
			}
		} else {
			// 内容不大，正常处理
			logger.Printf("[%s] 📝 内容大小正常 (%d 字符)，直接处理", requestID, len(execResultStr))

			// 将执行结果序列化为 JSON
			execResultJSON, _ := json.MarshalIndent(execResult, "", "  ")

			// 构造更清晰的执行结果消息
			toolResultMessage := fmt.Sprintf("工具调用 '%s' 的执行结果：\n\n```json\n%s\n```\n\n请基于以上结果继续回答用户的问题。", serverCall.Name, string(execResultJSON))

			// 更新 messages：添加 AI 的响应（包含前缀文本）和工具结果
			newMessages := make([]interface{}, len(messages))
			copy(newMessages, messages)

			// 添加 AI 的响应（只包含前缀文本，不包含 ）
			if prefixText != "" {
				newMessages = append(newMessages, map[string]interface{}{
					"role":    "assistant",
					"content": prefixText,
				})
			}

			// 添加工具结果
			newMessages = append(newMessages, map[string]interface{}{
				"role":    "user",
				"content": toolResultMessage,
			})

			// 更新 requestData
			requestData["messages"] = newMessages

			// 恢复原始的 role_name，确保再次调用时保持系统提示词类型
			if originalRoleName != "" {
				requestData["role_name"] = originalRoleName
				logger.Printf("[%s] 📌 恢复 role_name: %s", requestID, originalRoleName)
			}

			// system_prompt 处理已移至格式转换器中，此处不再处理

			logger.Printf("[%s] 📤 重新调用 AI，消息数: %d", requestID, len(newMessages))

			// 重新调用 AI（使用新的统一方法）
			response, err := s.CreateWebSocketNonStreamResponse(ctx, requestData, requestID)
			if err != nil {
				logger.Printf("[%s] ❌ 重新调用 AI 失败: %v", requestID, err)
				return "", fmt.Errorf("重新调用 AI 失败: %v", err)
			}

			// 从响应中提取文本内容
			var newResponse string
			if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := choice["message"].(map[string]interface{}); ok {
						if content, ok := message["content"].(string); ok {
							newResponse = content
						}
					}
				}
			}

			if newResponse == "" {
				logger.Printf("[%s] ❌ AI 返回了空响应", requestID)
				return "", fmt.Errorf("AI 返回了空响应")
			}

			logger.Printf("[%s] ✅ AI 重新响应成功，长度: %d", requestID, len(newResponse))
			logger.Printf("[%s] 📝 AI 响应内容预览: %s", requestID, truncateString(newResponse, 200))
			currentResponse = newResponse
			messages = newMessages
		}
	}

	// 达到最大迭代次数
	logger.Printf("[%s] ⚠️  达到最大执行循环次数 %d，返回最后响应", requestID, maxIterations)
	filteredResponse := s.filterServerCalls(currentResponse)

	// 如果启用了 TTS，在返回前进行语音合成
	if enableTTS && filteredResponse != "" && ttsCallback != nil {
		go s.synthesizeSpeechAsync(filteredResponse, voiceGender, requestID, ttsCallback)
	}

	return filteredResponse, nil
}
