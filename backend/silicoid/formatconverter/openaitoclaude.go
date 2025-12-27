package formatconverter

import (
	"fmt"
	"strings"
)

// RequestOpenAIToClaude 将OpenAI格式的请求转换为Claude Messages API格式
func (s *SilicoidFormatConverterService) RequestOpenAIToClaude(openaiRequest map[string]interface{}) (map[string]interface{}, error) {
	logger.Println("开始转换OpenAI请求为Claude Messages API格式")

	// 添加客户端执行器工具（如果角色支持）
	s.AddExecutorTools(openaiRequest)

	// 提取OpenAI请求中的关键信息
	messages, _ := openaiRequest["messages"].([]interface{})
	model, _ := openaiRequest["model"].(string)
	maxTokens, _ := openaiRequest["max_tokens"].(float64)
	if maxTokens == 0 {
		maxTokens = 2048
	}
	temperature, _ := openaiRequest["temperature"].(float64)
	if temperature == 0 {
		temperature = 0.7
	}
	topP, _ := openaiRequest["top_p"].(float64)
	if topP == 0 {
		topP = 1.0
	}
	stream, _ := openaiRequest["stream"].(bool)

	// 处理思考模式参数
	thinkingEnabled, _ := openaiRequest["thinking_enabled"].(bool)
	thinkingBudget, _ := openaiRequest["thinking_budget"].(float64)
	if thinkingBudget == 0 {
		thinkingBudget = 128000
	}

	// 处理工具相关参数
	tools, _ := openaiRequest["tools"].([]interface{})
	toolChoice, _ := openaiRequest["tool_choice"]
	toolResults, _ := openaiRequest["tool_results"].([]interface{})

	// 处理 system_prompt：从 Redis 获取并注入到请求中
	systemMessage := s.processSystemPrompt(openaiRequest)

	// 获取 userId（如果存在）
	userId, _ := openaiRequest["_user_id"].(string)
	if userId == "" {
		// 兼容调用方仅传递 user_id 的情况（如 WebSocket 路径）
		if uid, ok := openaiRequest["user_id"].(string); ok && uid != "" {
			userId = uid
		}
	}
	if userId == "" {
		// 再次兜底：少数路径可能使用 user 字段
		if uid, ok := openaiRequest["user"].(string); ok && uid != "" {
			userId = uid
		}
	}
	if userId != "" {
		logger.Printf("已解析到用户ID用于文件读取: %s", userId)
	} else {
		logger.Printf("未解析到用户ID，将无法执行 file_read 从用户文件读取")
	}

	// 初始化 Claude 消息列表
	var claudeMessages []map[string]interface{}

	// 处理OpenAI消息，转换为Claude消息格式
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		content := msgMap["content"]

		if role == "system" {
			// 处理 system 消息
			contentStr, _ := content.(string)
			if systemMessage != "" {
				logger.Println("⚠️  已处理 system_prompt，忽略 messages 中的 system 消息，避免重复")
			} else {
				systemMessage = contentStr
				logger.Println("使用 messages 中的 system 消息")
			}
		} else if role == "user" {
			// 处理 user 消息，支持字符串和数组格式
			var claudeContent interface{}
			
			switch contentVal := content.(type) {
			case string:
				// 字符串格式，直接使用
				if contentVal == "" {
					contentVal = "请继续"
				}
				claudeContent = contentVal
			case []interface{}:
				// 数组格式，需要处理文件等
				claudeContentArray, err := s.processContentArray(contentVal, userId)
				if err != nil {
					logger.Printf("处理 content 数组失败: %v", err)
					// 降级为文本
					claudeContent = "文件处理失败，请重试"
				} else {
					claudeContent = claudeContentArray
				}
			default:
				// 其他类型，转换为字符串
				claudeContent = fmt.Sprintf("%v", content)
			}

			claudeMessages = append(claudeMessages, map[string]interface{}{
				"role":    "user",
				"content": claudeContent,
			})
		} else if role == "assistant" {
			// 检查是否有工具调用信息
			toolCalls, hasToolCalls := msgMap["tool_calls"].([]interface{})
			if hasToolCalls && len(toolCalls) > 0 {
				logger.Println("发现带有工具调用的助手消息，将分离处理")
				// 仅添加文本内容
				if content != "" {
					claudeMessages = append(claudeMessages, map[string]interface{}{
						"role":    "assistant",
						"content": content,
					})
				}
			} else {
				claudeMessages = append(claudeMessages, map[string]interface{}{
					"role":    "assistant",
					"content": content,
				})
			}
		} else if role == "tool" {
			// 处理工具响应消息，将其作为用户消息添加
			toolName, _ := msgMap["name"].(string)
			toolContent := content
			formattedContent := fmt.Sprintf("工具 %s 的结果:\n%s", toolName, toolContent)
			claudeMessages = append(claudeMessages, map[string]interface{}{
				"role":    "user",
				"content": formattedContent,
			})
		}
	}

	// 如果消息列表为空，添加一个默认用户消息
	if len(claudeMessages) == 0 {
		claudeMessages = append(claudeMessages, map[string]interface{}{
			"role":    "user",
			"content": "Hello",
		})
	}

	// 确保消息列表符合Claude API要求
	// 检查列表中没有连续的助手消息
	var validClaudeMessages []map[string]interface{}
	var prevRole string

	for _, msg := range claudeMessages {
		currentRole, _ := msg["role"].(string)
		// 如果当前和前一个都是assistant，跳过当前消息
		if currentRole == "assistant" && prevRole == "assistant" {
			continue
		}
		// 添加有效消息
		validClaudeMessages = append(validClaudeMessages, msg)
		prevRole = currentRole
	}

	// 确保消息列表不以assistant结尾（除非是空内容）
	if len(validClaudeMessages) > 0 && validClaudeMessages[len(validClaudeMessages)-1]["role"] == "assistant" {
		// 如果最后一条消息是助手消息，且内容为空或只有空白字符
		lastContent, _ := validClaudeMessages[len(validClaudeMessages)-1]["content"].(string)
		if strings.TrimSpace(lastContent) == "" {
			// 更新为一个有意义的内容
			validClaudeMessages[len(validClaudeMessages)-1]["content"] = "我会继续帮助您。"
		} else {
			// 添加用户消息，请求继续
			validClaudeMessages = append(validClaudeMessages, map[string]interface{}{
				"role":    "user",
				"content": "请继续",
			})
		}
	}

	// 直接使用请求中的模型，不允许使用默认值或选择模型
	if model == "" {
		return nil, fmt.Errorf("模型参数不能为空")
	}

	// 构建Claude Messages API请求
	claudeRequest := map[string]interface{}{
		"model":        model,
		"messages":     validClaudeMessages,
		"max_tokens":   int(maxTokens),
		"temperature":  temperature,
		"top_p":        topP,
		"stream":       stream,
	}

	// 如果存在系统消息，添加为顶级system参数而不是消息列表中的系统角色消息
	if systemMessage != "" {
		claudeRequest["system"] = systemMessage
		logger.Printf("设置 Claude system 参数 (长度: %d)", len(systemMessage))
	}
	
	// 清理内部标记，不传递给 Claude API
	delete(claudeRequest, "_system_prompt")

	// 如果启用了思考模式，添加thinking参数
	if thinkingEnabled {
		// 确保max_tokens大于thinking_budget，按照Claude API的要求
		if float64(claudeRequest["max_tokens"].(int)) <= thinkingBudget {
			// 将max_tokens设置为thinking_budget + 1000，确保有足够空间生成回答
			claudeRequest["max_tokens"] = int(thinkingBudget) + 1000
			logger.Printf("已调整max_tokens为 %d，以确保大于思考预算 %d", claudeRequest["max_tokens"], int(thinkingBudget))
		}

		// 强制设置temperature为1，这是Claude API在思考模式下的要求
		if claudeRequest["temperature"].(float64) != 1.0 {
			logger.Printf("因思考模式已启用，temperature已从 %f 调整为 1.0", claudeRequest["temperature"])
			claudeRequest["temperature"] = 1.0
		}

		// 移除top_p参数，这是Claude API在思考模式下的要求
		if _, ok := claudeRequest["top_p"]; ok {
			logger.Printf("因思考模式已启用，已移除top_p参数 (原值: %f)", claudeRequest["top_p"])
			delete(claudeRequest, "top_p")
		}

		claudeRequest["thinking"] = map[string]interface{}{
			"type":          "enabled",
			"budget_tokens": int(thinkingBudget),
		}
		logger.Printf("启用思考模式，预算令牌数: %d", int(thinkingBudget))
	}

	// 处理工具（如果存在）
	if len(tools) > 0 {
		// 转换工具格式（从OpenAI格式到Claude格式）
		claudeTools, err := s.convertToolsToClaudeFormat(tools)
		if err != nil {
			return nil, err
		}
		claudeRequest["tools"] = claudeTools
		logger.Printf("添加了 %d 个工具到Claude请求", len(claudeTools))

		// 处理工具选择设置
		if toolChoice != nil {
			if toolChoiceStr, ok := toolChoice.(string); ok {
				if toolChoiceStr == "auto" || toolChoiceStr == "none" {
					claudeRequest["tool_choice"] = toolChoiceStr
				} else {
					claudeRequest["tool_choice"] = "auto"
				}
			} else if toolChoiceMap, ok := toolChoice.(map[string]interface{}); ok {
				if toolChoiceMap["type"] == "function" {
					// 为避免格式问题，暂时使用auto
					claudeRequest["tool_choice"] = "auto"
					logger.Println("收到复杂的工具选择对象，为兼容性改用'auto'")
				} else {
					claudeRequest["tool_choice"] = "auto"
				}
			} else {
				claudeRequest["tool_choice"] = "auto"
			}
			logger.Printf("设置工具使用模式: %v", claudeRequest["tool_choice"])
		} else {
			// 默认设置为auto
			claudeRequest["tool_choice"] = "auto"
			logger.Println("未指定工具选择，默认设为自动模式")
		}
	}

	// 处理工具结果（如果存在）
	if len(toolResults) > 0 {
		// 在消息末尾添加最后一条用户消息，包含工具结果摘要
		toolResultSummary := "以下是工具执行结果:\n\n"
		for _, result := range toolResults {
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				continue
			}
			toolID, _ := resultMap["id"].(string)
			output, _ := resultMap["output"].(string)
			toolResultSummary += fmt.Sprintf("结果 (ID: %s):\n%s\n\n", toolID, output)
		}

		// 确保消息列表最后一条是用户消息，包含工具结果
		messages := claudeRequest["messages"].([]map[string]interface{})
		if len(messages) > 0 && messages[len(messages)-1]["role"] == "assistant" {
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": toolResultSummary,
			})
		} else {
			// 添加包含工具结果的user消息
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": toolResultSummary,
			})
		}
		claudeRequest["messages"] = messages
		logger.Printf("添加了 %d 个工具结果到消息中", len(toolResults))
	}

	logger.Printf("转换完成: OpenAI -> Claude Messages API, 模型: %s", model)
	return claudeRequest, nil
}


// convertToolsToClaudeFormat 将OpenAI格式的工具转换为Claude格式
func (s *SilicoidFormatConverterService) convertToolsToClaudeFormat(openaiTools []interface{}) ([]map[string]interface{}, error) {
	var claudeTools []map[string]interface{}

	for _, tool := range openaiTools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		toolType, _ := toolMap["type"].(string)
		if toolType == "" {
			toolType = "function"
		}

		if toolType == "function" {
			functionData, ok := toolMap["function"].(map[string]interface{})
			if !ok {
				continue
			}

			// 创建基本的Claude工具结构
			claudeTool := map[string]interface{}{
				"name":        functionData["name"],
				"description": functionData["description"],
			}

			// 转换参数
			parameters, ok := functionData["parameters"].(map[string]interface{})
			if ok && len(parameters) > 0 {
				claudeTool["input_schema"] = parameters
			}

			claudeTools = append(claudeTools, claudeTool)
			logger.Printf("转换工具 %v 成功", claudeTool["name"])
		} else {
			logger.Printf("不支持的工具类型: %s，已跳过", toolType)
		}
	}

	return claudeTools, nil
}