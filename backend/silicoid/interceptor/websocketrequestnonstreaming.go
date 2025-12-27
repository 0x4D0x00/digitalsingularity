package interceptor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateWebSocketNonStreamResponse 创建WebSocket非流式 AI 响应
func (s *SilicoIDInterceptor) CreateWebSocketNonStreamResponse(ctx context.Context, requestData map[string]interface{}, requestID string) (map[string]interface{}, error) {
	// 生成 session_id
	sessionID := uuid.New().String()
	logger.Printf("[%s] 📝 生成 session_id: %s", requestID, sessionID)

	// 获取模型名称
	modelName, ok := requestData["model"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少模型参数")
	}

	// 获取用户ID和API Key
	userID, _ := requestData["user_id"].(string)
	apiKey, _ := requestData["api_key"].(string)

	// 检查用户资产
	hasAssets, err := s.checkUserAssets(userID, apiKey)
	if err != nil {
		return nil, fmt.Errorf("检查用户资产失败: %v", err)
	}
	if !hasAssets {
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": "用户资产不足",
				"type":    "insufficient_tokens",
				"code":    "insufficient_tokens",
			},
		}, nil
	}

	// 根据model_name查询模型配置（获取base_url和endpoint）
	modelConfig, err := s.modelManager.GetModelConfig(modelName)
	if err != nil {
		logger.Printf("[%s] ⚠️ 获取模型配置失败: %v", requestID, err)
		return nil, fmt.Errorf("获取模型配置失败: %v", err)
	}

	// 将获取到的配置存储到requestData中
	modelCode := modelConfig.ModelCode
	baseURL := modelConfig.BaseURL
	endpoint := modelConfig.Endpoint
	requestData["model_code"] = modelCode
	requestData["_base_url"] = baseURL
	requestData["_endpoint"] = endpoint
	logger.Printf("[%s] ✅ 成功获取模型配置: model_code=%s, base_url=%s, endpoint=%s",
		requestID, modelCode, baseURL, endpoint)

	// 判断是否是 Claude 模型
	isClaudeModel := s.isClaudeModel(modelName)

	logger.Printf("[%s] 📡 创建非流式响应，模型: %s, Claude模型: %v", requestID, modelName, isClaudeModel)

	// 保存原始的 role_name，防止被 formatConverter 删除
	originalRoleName, _ := requestData["role_name"].(string)
	logger.Printf("[%s] 📌 保存原始 role_name: %s", requestID, originalRoleName)

	// 为所有模型添加工具支持（包括MCP和客户端执行器工具）
	if err := s.formatConverter.AddExecutorTools(requestData); err != nil {
		logger.Printf("[%s] ⚠️ 添加执行器工具失败: %v", requestID, err)
	}

	var response map[string]interface{}

	if isClaudeModel {
		// 使用 Claude 服务
		logger.Printf("[%s] 使用 Claude 服务处理非流式请求", requestID)

		// 先将 OpenAI 格式转换为 Claude 格式
		claudeData, err := s.formatConverter.RequestOpenAIToClaude(requestData)
		if err != nil {
			return nil, fmt.Errorf("格式转换失败: %v", err)
		}

		claudeResponse := s.claudeService.CreateChatCompletionNonStream(ctx, claudeData)

		// 将 Claude 响应转换为 OpenAI 格式
		response, err = s.formatConverter.ResponseClaudeToOpenAI(claudeResponse, requestData)
		if err != nil {
			return nil, fmt.Errorf("响应格式转换失败: %v", err)
		}

		// 统一的错误检测和日志记录
		if checkAndLogResponseError(response, requestID, "Claude") {
			return response, nil // 返回错误响应，让上层处理
		}
	} else {
		// 使用 OpenAI 服务
		logger.Printf("[%s] 使用 OpenAI 兼容服务处理非流式请求", requestID)

		// 直接传递请求数据，让 OpenAI 服务自己处理规范化
		response = s.openaiService.CreateChatCompletionNonStream(ctx, requestData)

		// 统一的错误检测和日志记录
		if checkAndLogResponseError(response, requestID, "OpenAI") {
			return response, nil // 返回错误响应，让上层处理
		}
	}
	// 打印模型返回的原始非流式响应，便于排查格式/解析问题
	if response != nil {
		if respBytes, err := json.Marshal(response); err == nil {
			logger.Printf("[%s] RAW_MODEL_RESPONSE (len=%d): %s", requestID, len(respBytes), truncateString(string(respBytes), 2000))
		} else {
			logger.Printf("[%s] RAW_MODEL_RESPONSE marshal error: %v", requestID, err)
		}

		// 打印 choices[0].message.function_call（如果存在）
		if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
			if firstChoice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := firstChoice["message"].(map[string]interface{}); ok {
					if fc, ok := message["function_call"]; ok {
						if fcBytes, err := json.Marshal(fc); err == nil {
							logger.Printf("[%s] RAW_MODEL_FUNCTION_CALL: %s", requestID, truncateString(string(fcBytes), 1000))
						}
					}
				}
			}
		}

		// 打印 top-level tool_calls（如果存在）
		if tc, ok := response["tool_calls"]; ok {
			if tcBytes, err := json.Marshal(tc); err == nil {
				logger.Printf("[%s] RAW_MODEL_TOOL_CALLS: %s", requestID, truncateString(string(tcBytes), 1000))
			}
		}

		// 打印 session_id（如果存在）
		if sid, ok := response["session_id"].(string); ok && sid != "" {
			logger.Printf("[%s] MODEL_SESSION_ID: %s", requestID, sid)
		}
	}

	// 恢复原始的 role_name，确保后续 ServerCalls 处理时能正确识别
	if originalRoleName != "" {
		requestData["role_name"] = originalRoleName
		logger.Printf("[%s] 📌 恢复 role_name: %s", requestID, originalRoleName)
	}

	// 在响应中添加 session_id（优先使用大模型返回的 session_id）
	if response != nil {
		// 检查大模型响应中是否包含 session_id
		if modelSessionID, ok := response["session_id"].(string); ok && modelSessionID != "" {
			// 使用大模型返回的 session_id
			logger.Printf("[%s] ✅ 使用大模型返回的 session_id: %s", requestID, modelSessionID)
			sessionID = modelSessionID
			// 确保 response 中包含正确的 session_id
			response["session_id"] = sessionID
		} else {
			// 大模型没有提供 session_id，使用我们生成的
			response["session_id"] = sessionID
			logger.Printf("[%s] ✅ 大模型未提供 session_id，使用生成的: %s", requestID, sessionID)
		}
	}

	return response, nil
}

// HandleWebSocketRequestNonStream 处理所有模型的WebSocket非流式请求
// ProcessNonStreamChat 处理非流式AI聊天（WebSocket接口）
func (s *SilicoIDInterceptor) ProcessNonStreamChat(
	ctx context.Context,
	requestID string,
	userID string,
	requestData map[string]interface{},
	sendMessage func(messageType string, data map[string]interface{}) error,
) error {
	return s.HandleWebSocketRequestNonStream(ctx, requestID, userID, requestData, sendMessage)
}

func (s *SilicoIDInterceptor) HandleWebSocketRequestNonStream(
	ctx context.Context,
	requestID string,
	userID string,
	requestData map[string]interface{},
	sendMessage func(messageType string, data map[string]interface{}) error,
) error {
	logger.Printf("[%s] 处理AI聊天请求 (user=%s)", requestID, userID)

	// 发起AI请求
	response, err := s.CreateWebSocketNonStreamResponse(ctx, requestData, requestID)
	if err != nil {
		logger.Printf("[%s] AI请求失败: %v", requestID, err)
		return fmt.Errorf("AI请求失败: %v", err)
	}

	// 处理响应并发送消息
	return s.processWebSocketNonStreamResponse(ctx, response, requestID, userID, requestData, sendMessage)
}

// processWebSocketNonStreamResponse 处理WebSocket非流式响应（内部方法）
func (s *SilicoIDInterceptor) processWebSocketNonStreamResponse(
	ctx context.Context,
	response map[string]interface{},
	requestID string,
	userID string,
	requestData map[string]interface{},
	sendMessage func(messageType string, data map[string]interface{}) error,
) error {
	logger.Printf("[%s] 开始处理WebSocket非流式响应", requestID)

	// 检查是否有错误
	if errorData, hasError := response["error"]; hasError {
		errorMsg := fmt.Sprintf("AI API 错误: %v", errorData)
		logger.Printf("[%s] %s", requestID, errorMsg)
		return fmt.Errorf(errorMsg)
	}

	// 提取AI响应内容
	var fullResponse string
	if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					fullResponse = content
					logger.Printf("[%s] 获取AI响应，长度: %d", requestID, len(fullResponse))
				}
			}
		}
	}

	// 检查是否有结构化调用
	hasStructuredCalls := false
	if response != nil {
		if tcRaw, ok := response["tool_calls"]; ok {
			if tcArr, ok := tcRaw.([]interface{}); ok && len(tcArr) > 0 {
				hasStructuredCalls = true
			}
		}
		if !hasStructuredCalls {
			if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
				if firstChoice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := firstChoice["message"].(map[string]interface{}); ok {
						if _, ok := message["function_call"]; ok {
							hasStructuredCalls = true
						} else if _, ok := message["tool_calls"]; ok {
							hasStructuredCalls = true
						}
					}
				}
			}
		}
	}

	// 检查文本格式的工具调用
	textToolCalls := extractTextToolCalls(fullResponse)
	var calls []interface{}
	if len(textToolCalls) > 0 {
		logger.Printf("[%s] 检测到 %d 个文本格式工具调用", requestID, len(textToolCalls))
		hasStructuredCalls = true
		// 将文本工具调用转换为结构化格式
		calls = convertTextCallsToStructured(textToolCalls)
	}

	if hasStructuredCalls {
		// 提取结构化调用并判断执行类型
		logger.Printf("[%s] 检测到结构化工具调用，开始处理", requestID)

		// 使用 extractStructuredCallsFromResponse 判断哪些调用需要在服务器端执行
		serverCalls := s.extractStructuredCallsFromResponse(response, requestID)
		logger.Printf("[%s] extractStructuredCallsFromResponse 返回 %d 个服务器调用", requestID, len(serverCalls))

		if len(serverCalls) > 0 {
			// 有服务器端执行器调用，使用新的结构化处理流程
			logger.Printf("[%s] 发现 %d 个服务器端执行器调用，使用结构化服务器端处理流程", requestID, len(serverCalls))

			// 使用新的方法处理结构化的服务器调用
			finalResponse, err := s.ProcessAIResponseWithStructuredServerCalls(
				ctx, requestData, serverCalls, response, false, requestID, 5, false, "", nil, nil)

			if err != nil {
				logger.Printf("[%s] 结构化服务器端调用处理失败: %v", requestID, err)
				return fmt.Errorf("结构化服务器端调用处理失败: %v", err)
			}

			// 发送最终结果
			logger.Printf("[%s] 结构化服务器端调用处理完成，发送最终回答", requestID)

			finalData := map[string]interface{}{
				"type":      "chat_complete",
				"content":   finalResponse,
				"timestamp": time.Now().Unix(),
			}

			if requestID != "" {
				finalData["session_id"] = requestID
			}

			return sendMessage("chat_complete", finalData)
		}

		// 处理客户端调用（原有逻辑）
		logger.Printf("[%s] 处理客户端工具调用", requestID)

		// 如果还没有设置calls（不是文本调用），从响应中提取结构化调用
		if len(calls) == 0 {
			if tcRaw, ok := response["tool_calls"]; ok {
				if tcArr, ok := tcRaw.([]interface{}); ok {
					calls = tcArr
				}
			} else if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
				if firstChoice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := firstChoice["message"].(map[string]interface{}); ok {
						if fcRaw, ok := message["function_call"]; ok {
							calls = []interface{}{fcRaw}
						} else if tcRaw, ok := message["tool_calls"]; ok {
							if tcArr, ok := tcRaw.([]interface{}); ok {
								calls = tcArr
							}
						}
					}
				}
			}
		}

		if len(calls) > 0 {
			// 保存上下文到Redis
			go func() {
				// 构建AI响应数据
				var aiResponse map[string]interface{}
				if response != nil {
					if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
						if firstChoice, ok := choices[0].(map[string]interface{}); ok {
							if message, ok := firstChoice["message"].(map[string]interface{}); ok {
								aiResponse = map[string]interface{}{
									"role":    "assistant",
									"content": "",
								}
								if content, ok := message["content"].(string); ok {
									aiResponse["content"] = content
								}
								if toolCalls, ok := message["tool_calls"]; ok {
									aiResponse["tool_calls"] = toolCalls
								}
								if fc, ok := message["function_call"]; ok {
									aiResponse["function_call"] = fc
								}
							}
						}
					}
				}

				// 使用会话管理服务保存工具调用上下文
				err := s.SaveToolCallContext(userID, requestID, requestData, aiResponse)
				if err != nil {
					logger.Printf("[%s] 保存工具调用上下文失败: %v", requestID, err)
				} else {
					logger.Printf("[%s] 已保存工具调用上下文: session_id=%s", requestID, requestID)
				}
			}()

			clientCallData := map[string]interface{}{
				"type":  "client_executor_call",
				"calls": calls,
			}

			// 使用原始请求的requestID作为session_id，确保上下文连续性
			if requestID != "" {
				clientCallData["session_id"] = requestID
				logger.Printf("[%s] 设置client_executor_call的session_id: %s", requestID, requestID)
			} else if sid, ok := response["session_id"].(string); ok && sid != "" {
				clientCallData["session_id"] = sid
			}

			return sendMessage("client_executor_call", clientCallData)
		}
	} else {
		// 发送最终回答
		logger.Printf("[%s] 发送最终回答", requestID)

		// 分离THINK内容和最终回答
		thinkContent := s.ExtractThinkContent(fullResponse)
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
		cleanContent := s.RemoveThinkTags(fullResponse)
		finalData := map[string]interface{}{
			"type":      "chat_complete",
			"content":   cleanContent,
			"timestamp": time.Now().Unix(),
		}

		if sid, ok := response["session_id"].(string); ok && sid != "" {
			finalData["session_id"] = sid
		}

		return sendMessage("chat_complete", finalData)
	}

	return nil
}
