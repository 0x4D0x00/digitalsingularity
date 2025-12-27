package interceptor

import (
	"fmt"
	"net/http"
	"github.com/gin-gonic/gin"
)

// CreateHTTPNonStreamResponse 创建HTTP非流式AI响应
func (s *SilicoIDInterceptor) CreateHTTPNonStreamResponse(c *gin.Context, requestID string, userID string, data map[string]interface{}) (map[string]interface{}, error) {
	// 获取模型名称
	modelName, ok := data["model"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少模型参数")
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
	data["model_code"] = modelCode
	data["_base_url"] = baseURL
	data["_endpoint"] = endpoint
	logger.Printf("[%s] ✅ 成功获取模型配置: model_code=%s, base_url=%s, endpoint=%s",
		requestID, modelCode, baseURL, endpoint)

	// 判断是否是 Claude 模型
	isClaudeModel := s.isClaudeModel(modelName)
	logger.Printf("[%s] 创建HTTP非流式响应，模型: %s, Claude模型: %v", requestID, modelName, isClaudeModel)

	// 将 userId 添加到请求数据中，供 formatconverter 使用
	data["_user_id"] = userID

	if isClaudeModel {
		// 处理Claude非流式请求
		logger.Printf("[%s] 处理Claude非流式请求", requestID)

	// 检查并纠正模型名称
		if modelName == "claude-3-7-sonnet-20250222" {
		data["model"] = "claude-3-7-sonnet-20250219"
		logger.Printf("[%s] 模型名称更正: claude-3-7-sonnet-20250222 -> claude-3-7-sonnet-20250219", requestID)
	}

	// 处理思考模式参数
	thinkingEnabled, _ := data["thinking_enabled"].(bool)
		if thinkingEnabled {
	thinkingBudget := 16000
		if budget, ok := data["thinking_budget"].(float64); ok {
			thinkingBudget = int(budget)
		}
		logger.Printf("[%s] 已启用Claude思考模式，预算令牌数: %d", requestID, thinkingBudget)
	}


		// ServerCalls 循环处理 - 使用新的分批处理机制
		const maxServerCallsIterations = 5
		iteration := 0
		var response map[string]interface{}

		// 获取原始 messages (OpenAI 格式) 并确保都有 id
		messages, _ := data["messages"].([]interface{})
		messages = ensureMessagesHaveID(messages)

		for iteration < maxServerCallsIterations {
			iteration++
			logger.Printf("[%s] 📍 ServerCalls 循环第 %d 次", requestID, iteration)

			// 更新请求数据并转换为 Claude 格式
			data["messages"] = messages
			claudeData, err := s.formatConverter.RequestOpenAIToClaude(data)
			if err != nil {
				logger.Printf("[%s] OpenAI转Claude格式转换失败: %v", requestID, err)
				return nil, fmt.Errorf("模型格式转换失败: %v", err)
			}

			// 调用Claude API
			claudeResponse := s.claudeService.CreateChatCompletionNonStream(c.Request.Context(), claudeData)

			// 将Claude响应转换为OpenAI格式
			response, err = s.formatConverter.ResponseClaudeToOpenAI(claudeResponse, data)
			if err != nil {
				logger.Printf("[%s] Claude转OpenAI格式转换失败: %v", requestID, err)
				return nil, fmt.Errorf("响应格式转换失败: %v", err)
			}

			// 检查是否有错误
			if errObj, exists := response["error"]; exists {
				logger.Printf("[%s] Claude API 返回错误: %v", requestID, errObj)
				break
			}

			// 提取助手的回复
			choices, _ := response["choices"].([]interface{})
			if len(choices) == 0 {
				logger.Printf("[%s] 没有收到响应内容", requestID)
				break
			}

			firstChoice, _ := choices[0].(map[string]interface{})
			message, _ := firstChoice["message"].(map[string]interface{})
			content, _ := message["content"].(string)

			logger.Printf("[%s] 📝 AI 响应内容长度: %d", requestID, len(content))

			// 优先尝试从结构化响应中解析工具调用（function_call / tool_calls）
			serverCalls := s.extractStructuredCallsFromResponse(response, requestID)

			if len(serverCalls) == 0 {
				logger.Printf("[%s] ✅ 没有检测到 ServerCalls 调用，结束循环", requestID)
				break
			}

			logger.Printf("[%s] 🔍 检测到 %d 个 ServerCalls 调用", requestID, len(serverCalls))

			// 执行所有 ServerCalls 调用并处理分批
			for _, call := range serverCalls {
				result, err := s.executeServerCall(c.Request.Context(), call, requestID)
				if err != nil {
					result = fmt.Sprintf("执行失败: %v", err)
				}

				// 检查是否需要分批处理
				const maxContentSize = 100000 // 10万字符限制

				if isLargeContent(result, maxContentSize) {
					logger.Printf("[%s] 📦 检测到大内容 (%d 字符)，开始分批处理", requestID, len(result))

					// 分批处理大内容
					chunks := chunkContent(result, call.Name, maxContentSize)
					logger.Printf("[%s] 📦 内容已分为 %d 个批次", requestID, len(chunks))

					// 使用改进的分批投喂机制（Claude版本）
					feedResults := make([]*BatchFeedResult, 0, len(chunks))

					// 将助手的回复（包含 ServerCalls 调用）添加到消息历史（只在第一批次时添加）
					messages = append(messages, map[string]interface{}{
						"id":      generateMessageID(),
						"role":    "assistant",
						"content": content,
					})

					// 分批投喂给AI，带重试机制
					for i, chunk := range chunks {
						logger.Printf("[%s] 📤 开始投喂第 %d/%d 批次内容 (长度: %d)", requestID, i+1, len(chunks), len(chunk.Content))

						// 使用重试机制投喂单个数据块（Claude版本）
						feedResult := s.feedBatchWithRetryClaude(c.Request.Context(), chunk, requestID, data, 3)
						feedResults = append(feedResults, feedResult)

						if !feedResult.Success {
							logger.Printf("[%s] ❌ 第 %d 批次投喂失败，但继续处理后续批次", requestID, chunk.Index)
							// 继续处理后续批次，不中断整个流程
						}
					}

					// 验证所有数据块是否都已投喂
					if err := s.validateAllBatchesFed(feedResults, requestID); err != nil {
						logger.Printf("[%s] ⚠️ 数据块投喂验证失败: %v", requestID, err)
						// 可以选择继续或返回错误，这里选择继续
					}

					// 如果是最后一批，让AI基于所有数据回答
					if len(chunks) > 0 {
						lastChunk := chunks[len(chunks)-1]
						if lastChunk.IsLast {
							logger.Printf("[%s] 📤 所有批次已投喂完毕，让AI基于所有数据回答", requestID)

							// 更新请求数据并转换为 Claude 格式
							data["messages"] = messages
							claudeData, err := s.formatConverter.RequestOpenAIToClaude(data)
							if err != nil {
								logger.Printf("[%s] 最终回答格式转换失败: %v", requestID, err)
								return nil, fmt.Errorf("最终回答格式转换失败: %v", err)
							}

							// 调用Claude API最终回答
							finalClaudeResponse := s.claudeService.CreateChatCompletionNonStream(c.Request.Context(), claudeData)

							// 将Claude响应转换为OpenAI格式
							response, err = s.formatConverter.ResponseClaudeToOpenAI(finalClaudeResponse, data)
							if err != nil {
								logger.Printf("[%s] 最终回答响应转换失败: %v", requestID, err)
								return nil, fmt.Errorf("最终回答响应转换失败: %v", err)
							}

							// 提取最终响应
							if finalChoices, ok := response["choices"].([]interface{}); ok && len(finalChoices) > 0 {
								if finalChoice, ok := finalChoices[0].(map[string]interface{}); ok {
									if finalMessage, ok := finalChoice["message"].(map[string]interface{}); ok {
										if finalContent, ok := finalMessage["content"].(string); ok {
											logger.Printf("[%s] ✅ AI 最终响应成功，长度: %d", requestID, len(finalContent))

											// 更新content为最终响应
											content = finalContent
										}
									}
								}
							}
						}
					}
				} else {
					// 内容不大，正常处理
					logger.Printf("[%s] 📝 内容大小正常 (%d 字符)，直接处理", requestID, len(result))

					// 将助手的回复（包含 ServerCalls 调用）添加到消息历史
					messages = append(messages, map[string]interface{}{
						"id":      generateMessageID(),
						"role":    "assistant",
						"content": content,
					})

					// 将 ServerCalls 执行结果添加到消息历史
					messages = s.appendServerCallResultToMessages(messages, []ServerCall{call}, []string{result})
				}
			}

			logger.Printf("[%s] 📌 将 ServerCalls 结果追加到消息历史，继续下一轮", requestID)
		}

		if iteration >= maxServerCallsIterations {
			logger.Printf("[%s] ⚠️  达到最大 ServerCalls 循环次数 (%d)，停止", requestID, maxServerCallsIterations)
		}

		// 确保最终响应对应的 assistant 消息已添加到 messages 历史中
		// 如果还没有添加，则添加它（这种情况发生在 ServerCalls 循环结束时，最终响应没有 ServerCalls 调用）
		if response != nil {
			if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
				if firstChoice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := firstChoice["message"].(map[string]interface{}); ok {
						if content, ok := message["content"].(string); ok && content != "" {
							// 检查 messages 历史中最后一条消息是否已经是这条消息
							needAdd := true
							if len(messages) > 0 {
								if lastMsg, ok := messages[len(messages)-1].(map[string]interface{}); ok {
									if lastRole, _ := lastMsg["role"].(string); lastRole == "assistant" {
										if lastContent, _ := lastMsg["content"].(string); lastContent == content {
											needAdd = false
										}
									}
								}
							}

							// 如果需要添加，则添加到 messages 历史中
							if needAdd {
								msgID := generateMessageID()
								// 如果响应中的 message 已经有 id，使用它；否则使用新生成的 id
								if existingID, hasID := message["id"].(string); hasID && existingID != "" {
									msgID = existingID
								} else {
									message["id"] = msgID
									firstChoice["message"] = message
									choices[0] = firstChoice
									response["choices"] = choices
								}

								messages = append(messages, map[string]interface{}{
									"id":      msgID,
									"role":    "assistant",
									"content": content,
								})
								logger.Printf("[%s] ✅ 已将最终响应添加到 messages 历史，id: %s", requestID, msgID)
							}
						}
					}
				}
			}
		}

		// 扣除token（如果需要）
		s.deductTokensIfNeeded(response, userID, requestID)

		logger.Printf("[%s] Claude请求完成", requestID)

		// 统一的错误检测和日志记录
		if checkAndLogResponseError(response, requestID, "Claude") {
			return response, nil // 返回错误响应，让上层处理
		}

		// 过滤响应中的服务端调用，并确保 message 有 id
		filteredResponse := s.filterServerCallsInResponse(response, messages)
		return filteredResponse, nil
	} else {
		// 处理OpenAI非流式请求
		logger.Printf("[%s] 处理OpenAI非流式请求", requestID)

		// 使用格式转换器规范化 OpenAI 请求
		normalizedData, err := s.formatConverter.NormalizeOpenAIRequest(data)
		if err != nil {
			logger.Printf("[%s] OpenAI 请求规范化失败: %v", requestID, err)
			return nil, fmt.Errorf("请求格式规范化失败: %v", err)
		}
		logger.Printf("[%s] OpenAI 请求已规范化", requestID)

		// 使用规范化后的数据
		data = normalizedData

		// 调用OpenAI API
		response := s.openaiService.CreateChatCompletionNonStream(c.Request.Context(), data)

		// 获取原始 messages 并确保都有 id（用于过滤响应）
		messages, _ := data["messages"].([]interface{})
		messages = ensureMessagesHaveID(messages)

		// 扣除token（如果需要）
		s.deductTokensIfNeeded(response, userID, requestID)

		logger.Printf("[%s] OpenAI请求完成", requestID)

		// 统一的错误检测和日志记录
		if checkAndLogResponseError(response, requestID, "OpenAI") {
			return response, nil // 返回错误响应，让上层处理
		}

		// 过滤响应中的服务端调用，并确保 message 有 id
		filteredResponse := s.filterServerCallsInResponse(response, messages)
		return filteredResponse, nil
	}
}

// HandleHTTPRequestNonStream 处理所有模型的HTTP非流式请求
func (s *SilicoIDInterceptor) HandleHTTPRequestNonStream(c *gin.Context, requestID string, userID string, data map[string]interface{}) {
	logger.Printf("[%s] 处理HTTP非流式请求", requestID)

	// 创建AI响应
	response, err := s.CreateHTTPNonStreamResponse(c, requestID, userID, data)
	if err != nil {
		logger.Printf("[%s] 创建HTTP非流式响应失败: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "创建响应失败: " + err.Error(),
				"type":    "internal_error",
				"code":    "response_creation_failed",
			},
		})
		return
	}

	// 处理响应并返回
	err = s.processHTTPNonStreamResponse(c, response, requestID)
	if err != nil {
		logger.Printf("[%s] 处理HTTP非流式响应失败: %v", requestID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "处理响应失败: " + err.Error(),
				"type":    "internal_error",
				"code":    "response_processing_failed",
			},
		})
		return
	}
}

// processHTTPNonStreamResponse 处理HTTP非流式响应（内部方法）
func (s *SilicoIDInterceptor) processHTTPNonStreamResponse(c *gin.Context, response map[string]interface{}, requestID string) error {
	logger.Printf("[%s] 开始处理HTTP非流式响应", requestID)

	// 检查是否有错误
	if errorData, hasError := response["error"]; hasError {
		logger.Printf("[%s] AI响应包含错误: %v", requestID, errorData)
		c.JSON(http.StatusBadRequest, response)
		return nil
	}

	// 返回成功响应
	logger.Printf("[%s] HTTP非流式请求处理完成", requestID)
	c.JSON(http.StatusOK, response)
	return nil
}
