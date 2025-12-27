// Claude API服务：处理Claude模型的请求

package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/silicoid/models/manager"
)

// 获取logger
var logger = log.New(log.Writer(), "claude_service: ", log.LstdFlags)

// ClaudeKeyManager Claude API密钥管理器，使用三层架构管理API密钥
type ClaudeKeyManager struct {
	modelManager *manager.ModelManager
	currentIndex int
	baseURL      string
}

// NewClaudeKeyManager 创建Claude密钥管理器实例
func NewClaudeKeyManager() *ClaudeKeyManager {
	mgr := &ClaudeKeyManager{
		modelManager: manager.NewModelManager(),
		currentIndex: 0,
		baseURL:      "https://api.anthropic.com",
	}

	logger.Printf("初始化Claude密钥管理器 (使用三层架构)")
	return mgr
}

// GetNextKey 获取下一个可用的API密钥
func (m *ClaudeKeyManager) GetNextKey() string {
	// 从模型管理器获取可用密钥列表 (已按优先级排序)
	apiKeys, err := m.modelManager.GetAvailableAPIKeys("Claude")
	if err != nil {
		logger.Printf("获取API密钥失败: %v", err)
		return ""
	}

	if len(apiKeys) == 0 {
		logger.Print("没有可用的API密钥")
		return ""
	}

	// 使用轮询策略选择密钥
	key := apiKeys[m.currentIndex%len(apiKeys)]
	m.currentIndex = (m.currentIndex + 1) % len(apiKeys)

	logger.Printf("使用API密钥: %s (优先级: %d)", key.KeyName, key.Priority)
	return key.APIKey
}

// GetKeyWithID 获取API密钥及其ID (用于后续状态更新)
func (m *ClaudeKeyManager) GetKeyWithID() (string, int, error) {
	apiKeys, err := m.modelManager.GetAvailableAPIKeys("Claude")
	if err != nil {
		return "", 0, err
	}

	if len(apiKeys) == 0 {
		return "", 0, fmt.Errorf("没有可用的API密钥")
	}

	key := apiKeys[m.currentIndex%len(apiKeys)]
	m.currentIndex = (m.currentIndex + 1) % len(apiKeys)

	return key.APIKey, key.ID, nil
}

// MarkKeyUnavailable 标记一个API密钥为不可用状态
func (m *ClaudeKeyManager) MarkKeyUnavailable(key string) {
	// 实际的失败状态会在调用API后通过UpdateKeyStatus更新
	logger.Printf("API密钥调用失败: %s...", key[:min(8, len(key))])
}

// UpdateKeyStatus 更新密钥状态
func (m *ClaudeKeyManager) UpdateKeyStatus(keyID int, success bool, errMsg string) error {
	return m.modelManager.UpdateKeyStatus(keyID, success, errMsg)
}

// GetBaseURL 获取基础URL
func (m *ClaudeKeyManager) GetBaseURL() string {
	// 尝试从模型管理器获取
	config, err := m.modelManager.GetModelConfig("Claude")
	if err == nil && config.BaseURL != "" {
		return config.BaseURL
	}
	// 降级到默认值
	return m.baseURL
}

// ClaudeService Claude服务
type ClaudeService struct {
	readWrite     *datahandle.CommonReadWriteService
	keyManager    *ClaudeKeyManager
	currentAPIKey string
	currentKeyID  int
	defaultModel  string
}

// NewClaudeService 创建Claude服务实例
func NewClaudeService() *ClaudeService {
	readWrite, _ := datahandle.NewCommonReadWriteService("database")
	keyManager := NewClaudeKeyManager()

	service := &ClaudeService{
		readWrite:    readWrite,
		keyManager:   keyManager,
		defaultModel: "claude-3-7-sonnet-20250219",
	}

	// 注意：如果将来 Claude 服务需要创建 FormatConverter 实例，
	// 也应该设置 MCP 管理器以支持 MCP 工具集成
	// 示例代码（当前未使用）：
	// if formatConverter := createFormatConverterIfNeeded(); formatConverter != nil {
	//     if mcpManager := getGlobalMCPManager(); mcpManager != nil {
	//         formatConverter.SetMCPManager(mcpManager)
	//         logger.Printf("✅ 已设置MCP管理器到Claude服务的格式转换器")
	//     }
	// }

	logger.Printf("初始化Claude服务完成 (三层架构)")
	return service
}

// initAPIKey 初始化API密钥
func (s *ClaudeService) initAPIKey() bool {
	key, keyID, err := s.keyManager.GetKeyWithID()
	if err != nil {
		logger.Printf("无法获取可用的API密钥: %v", err)
		s.currentAPIKey = ""
		s.currentKeyID = 0
		return false
	}

	// 设置当前API密钥
	s.currentAPIKey = key
	s.currentKeyID = keyID
	logger.Printf("成功初始化Claude API密钥 (ID: %d)", keyID)
	return true
}

// CreateChatCompletionNonStream 创建聊天完成请求
func (s *ClaudeService) CreateChatCompletionNonStream(ctx context.Context, data map[string]interface{}) map[string]interface{} {
	model, _ := data["model"].(string)
	if model == "" {
		model = s.defaultModel
	}
	
	// 检查是否使用用户自己的 Claude API Key（拦截器设置的标记）
	useUserKey, _ := data["_use_user_key"].(bool)
	userClaudeKey, _ := data["_user_claude_key"].(string)
	
	// 获取 model_code 用于查询 API 密钥（如果使用平台 Key）
	var modelCode string
	if !useUserKey {
		modelCode, _ = data["model_code"].(string)
		if modelCode == "" {
			modelCode = "Claude" // 默认使用 Claude
			logger.Printf("DEBUG: model_code 为空，使用默认值: %s", modelCode)
		} else {
			logger.Printf("DEBUG: 从参数中获取的 model_code: '%s'", modelCode)
		}
	}
	
	// 清理内部标记
	delete(data, "_use_user_key")
	delete(data, "_user_claude_key")
	delete(data, "model_code")
	
	messages, _ := data["messages"].([]interface{})
	maxTokens, _ := data["max_tokens"].(float64)
	if maxTokens == 0 {
		maxTokens = 1000
	}
	
	temperature, _ := data["temperature"].(float64)
	if temperature == 0 {
		temperature = 0.7
	}
	
	topP, _ := data["top_p"].(float64)
	if topP == 0 {
		topP = 1.0
	}
	
	systemMessage, _ := data["system"].(string)
	thinking, _ := data["thinking"].(map[string]interface{})
	
	logger.Printf("发送Claude Messages API请求，模型: %s", model)
	
	// 检查消息内容，确保没有空白消息
	validMessages := []map[string]interface{}{}
	for _, msg := range messages {
		message, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		
		// 确保消息内容不为空且不只包含空白字符
		content, _ := message["content"].(string)
		if content != "" && strings.TrimSpace(content) != "" {
			validMessages = append(validMessages, message)
		} else {
			// 用有效内容替换空白消息
			role, _ := message["role"].(string)
			if role == "user" {
				validMessages = append(validMessages, map[string]interface{}{
					"role":    "user",
					"content": "请继续",
				})
			} else if role == "assistant" {
				validMessages = append(validMessages, map[string]interface{}{
					"role":    "assistant",
					"content": "我会继续帮助您。",
				})
			}
		}
	}
	
	// 如果所有消息都被过滤掉了，添加一个默认用户消息
	if len(validMessages) == 0 {
		validMessages = append(validMessages, map[string]interface{}{
			"role":    "user",
			"content": "你好，请帮助我。",
		})
	}
	
	// 确保消息列表不以助手消息结尾
	lastMsg := validMessages[len(validMessages)-1]
	if lastMsg["role"] == "assistant" {
		validMessages = append(validMessages, map[string]interface{}{
			"role":    "user",
			"content": "请继续",
		})
	}
	
	// 更新消息列表
	messages = make([]interface{}, len(validMessages))
	for i, msg := range validMessages {
		messages[i] = msg
	}
	
	// 检查思考模式和maxTokens的兼容性
	if thinking != nil {
		thinkingType, _ := thinking["type"].(string)
		if thinkingType == "enabled" {
			thinkingBudget, _ := thinking["budget_tokens"].(float64)
			if thinkingBudget == 0 {
				thinkingBudget = 16000
			}
			
			if maxTokens <= thinkingBudget {
				// 调整maxTokens以确保大于thinking_budget
				maxTokens = thinkingBudget + 1000
				logger.Printf("已调整max_tokens为 %d，以确保大于思考预算 %d", int(maxTokens), int(thinkingBudget))
			}
			
			// 强制设置temperature为1，这是Claude API在思考模式下的要求
			if temperature != 1.0 {
				logger.Printf("因思考模式已启用，temperature已从 %f 调整为 1.0", temperature)
				temperature = 1.0
			}
			
			// 思考模式下不能设置top_p
			if topP != 0 {
				logger.Printf("因思考模式已启用，已移除top_p参数 (原值: %f)", topP)
				topP = 0
			}
		}
	}
	
	// 确定使用的 API Key
	switch {
	case useUserKey && userClaudeKey != "":
		// 使用用户自己的 Claude API Key
		s.currentAPIKey = userClaudeKey
		s.currentKeyID = 0
		logger.Printf("使用用户自己的 Claude API Key (模型: %s)", model)
		
	default:
		// 使用平台的 Claude API Key，根据 model_code 从数据库获取
		if modelCode == "" {
			modelCode = "Claude" // 默认值
		}
		
		apiKeys, err := s.keyManager.modelManager.GetAvailableAPIKeys(modelCode)
		if err != nil || len(apiKeys) == 0 {
			logger.Printf("获取平台API密钥失败 (模型代码: %s, 模型名: %s): %v", modelCode, model, err)
			return map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("无法获取API密钥: %v", err),
					"type":    "internal_error",
				},
			}
		}
		
		// 使用第一个可用的 API Key（已按优先级排序）
		s.currentAPIKey = apiKeys[0].APIKey
		s.currentKeyID = apiKeys[0].ID
		logger.Printf("使用平台 Claude API Key (模型代码: %s, 模型名: %s, KeyID: %d)", modelCode, model, apiKeys[0].ID)
	}
	
	// 获取可用密钥数量（用于重试）
	maxAttempts := 1 // 默认尝试一次
	if !useUserKey && modelCode != "" {
		apiKeys, _ := s.keyManager.modelManager.GetAvailableAPIKeys(modelCode)
		maxAttempts = len(apiKeys)
		if maxAttempts == 0 {
			maxAttempts = 1 // 至少尝试一次
		}
	}
	
	// 尝试所有可用的API密钥
	for attempt := 0; attempt < maxAttempts; attempt++ {
		
		// 准备请求参数
		messageParams := map[string]interface{}{
			"model":       model,
			"messages":    messages,
			"max_tokens":  int(maxTokens),
			"temperature": temperature,
		}
		
		// 只有在非思考模式下才添加top_p参数
		if thinking == nil || thinking["type"] != "enabled" {
			if topP != 0 {
				messageParams["top_p"] = topP
			}
		}
		
		// 添加系统消息（如果存在）
		if systemMessage != "" {
			messageParams["system"] = systemMessage
		}
		
		// 添加思考模式（如果启用）
		if thinking != nil {
			messageParams["thinking"] = thinking
			logger.Printf("已启用思考模式: %v", thinking)
		}
		
		// 记录完整的请求参数（不含敏感信息）
		requestLog := make(map[string]interface{})
		for k, v := range messageParams {
			if k == "messages" {
				requestLog[k] = fmt.Sprintf("[%d messages]", len(messages))
			} else {
				requestLog[k] = v
			}
		}
		requestLogJSON, _ := json.Marshal(requestLog)
		logger.Printf("完整请求参数: %s", string(requestLogJSON))
		
		// 记录请求开始时间
		startTime := time.Now()
		
		// 使用Anthropic Messages API创建聊天完成
		response, err := s.sendRequest(ctx, messageParams)
		
		// 处理错误
		if err != nil {
			// 更新密钥失败状态
			if s.currentKeyID > 0 {
				s.keyManager.UpdateKeyStatus(s.currentKeyID, false, err.Error())
			}
			
			// 记录当前使用的API密钥
			if s.currentAPIKey != "" {
				keyPreview := s.currentAPIKey
				if len(keyPreview) > 12 {
					keyPreview = keyPreview[:8] + "..." + keyPreview[len(keyPreview)-4:]
				} else {
					keyPreview = "***"
				}
				logger.Printf("使用API密钥 %s 请求失败: %v", keyPreview, err)
				
				// 标记当前密钥为不可用
				s.keyManager.MarkKeyUnavailable(s.currentAPIKey)
			}
			
			// 尝试使用下一个密钥
			logger.Printf("尝试使用下一个API密钥 (尝试 %d/%d)", attempt+1, maxAttempts)
			if !s.initAPIKey() {
				logger.Print("无法获取下一个可用的API密钥")
				return map[string]interface{}{
					"error": "所有API密钥都已耗尽",
				}
			}
			
			continue
		}
		
		// 更新密钥成功状态
		if s.currentKeyID > 0 {
			s.keyManager.UpdateKeyStatus(s.currentKeyID, true, "")
		}
		
		// 记录请求时间
		elapsedTime := time.Since(startTime)
		logger.Printf("Claude响应时间: %.2f秒", elapsedTime.Seconds())
		
		logger.Printf("Claude Messages API响应成功，内容长度: %d", len(fmt.Sprintf("%v", response)))
		
		return response
	}
	
	// 如果所有密钥都尝试过仍然失败
	return map[string]interface{}{
		"error": "所有API密钥都已尝试，但请求仍然失败",
	}
}

// CreateChatCompletionStream 创建流式聊天完成请求
func (s *ClaudeService) CreateChatCompletionStream(ctx context.Context, data map[string]interface{}) chan string {
	outputChan := make(chan string)
	
	go func() {
		defer close(outputChan)
		
		model, _ := data["model"].(string)
		if model == "" {
			model = s.defaultModel
		}
		
		messages, _ := data["messages"].([]interface{})
		maxTokens, _ := data["max_tokens"].(float64)
		if maxTokens == 0 {
			maxTokens = 1000
		}
		
		temperature, _ := data["temperature"].(float64)
		if temperature == 0 {
			temperature = 0.7
		}
		
		topP, _ := data["top_p"].(float64)
		if topP == 0 {
			topP = 1.0
		}
		
		systemMessage, _ := data["system"].(string)
		thinking, _ := data["thinking"].(map[string]interface{})
		
		logger.Printf("发送Claude Messages API流式请求，模型: %s", model)
		
		// 检查消息内容，确保没有空白消息
		validMessages := []map[string]interface{}{}
		for _, msg := range messages {
			message, ok := msg.(map[string]interface{})
			if !ok {
				continue
			}
			
			// 确保消息内容不为空且不只包含空白字符
			content, _ := message["content"].(string)
			if content != "" && strings.TrimSpace(content) != "" {
				validMessages = append(validMessages, message)
			} else {
				// 用有效内容替换空白消息
				role, _ := message["role"].(string)
				if role == "user" {
					validMessages = append(validMessages, map[string]interface{}{
						"role":    "user",
						"content": "请继续",
					})
				} else if role == "assistant" {
					validMessages = append(validMessages, map[string]interface{}{
						"role":    "assistant",
						"content": "我会继续帮助您。",
					})
				}
			}
		}
		
		// 如果所有消息都被过滤掉了，添加一个默认用户消息
		if len(validMessages) == 0 {
			validMessages = append(validMessages, map[string]interface{}{
				"role":    "user",
				"content": "你好，请帮助我。",
			})
		}
		
		// 确保消息列表不以助手消息结尾
		lastMsg := validMessages[len(validMessages)-1]
		if lastMsg["role"] == "assistant" {
			validMessages = append(validMessages, map[string]interface{}{
				"role":    "user",
				"content": "请继续",
			})
		}
		
		// 更新消息列表
		messages = make([]interface{}, len(validMessages))
		for i, msg := range validMessages {
			messages[i] = msg
		}
		
		// 检查思考模式和maxTokens的兼容性
		if thinking != nil {
			thinkingType, _ := thinking["type"].(string)
			if thinkingType == "enabled" {
				thinkingBudget, _ := thinking["budget_tokens"].(float64)
				if thinkingBudget == 0 {
					thinkingBudget = 16000
				}
				
				if maxTokens <= thinkingBudget {
					// 调整maxTokens以确保大于thinking_budget
					maxTokens = thinkingBudget + 1000
					logger.Printf("已调整max_tokens为 %d，以确保大于思考预算 %d", int(maxTokens), int(thinkingBudget))
				}
				
				// 强制设置temperature为1，这是Claude API在思考模式下的要求
				if temperature != 1.0 {
					logger.Printf("因思考模式已启用，temperature已从 %f 调整为 1.0", temperature)
					temperature = 1.0
				}
				
				// 思考模式下不能设置top_p
				if topP != 0 {
					logger.Printf("因思考模式已启用，已移除top_p参数 (原值: %f)", topP)
					topP = 0
				}
			}
		}
		
		// 检查是否使用用户自己的 Claude API Key（拦截器设置的标记）
		useUserKey, _ := data["_use_user_key"].(bool)
		userClaudeKey, _ := data["_user_claude_key"].(string)
		
		// 获取 model_code 用于查询 API 密钥（如果使用平台 Key）
		var modelCode string
		if !useUserKey {
			modelCode, _ = data["model_code"].(string)
			if modelCode == "" {
				modelCode = "Claude" // 默认使用 Claude
				logger.Printf("DEBUG: 流式请求 model_code 为空，使用默认值: %s", modelCode)
			} else {
				logger.Printf("DEBUG: 流式请求从参数中获取的 model_code: '%s'", modelCode)
			}
		}
		
		// 确定使用的 API Key
		var currentAPIKey string
		var currentKeyID int
		
		switch {
		case useUserKey && userClaudeKey != "":
			// 使用用户自己的 Claude API Key
			currentAPIKey = userClaudeKey
			currentKeyID = 0
			logger.Printf("流式请求使用用户自己的 Claude API Key (模型: %s)", model)
			
		default:
			// 使用平台的 Claude API Key，根据 model_code 从数据库获取
			if modelCode == "" {
				modelCode = "Claude" // 默认值
			}
			
			apiKeys, err := s.keyManager.modelManager.GetAvailableAPIKeys(modelCode)
			if err != nil || len(apiKeys) == 0 {
				logger.Printf("流式请求获取平台API密钥失败 (模型代码: %s, 模型名: %s): %v", modelCode, model, err)
				outputChan <- createErrorChunk(fmt.Sprintf("无法获取API密钥: %v", err))
				return
			}
			
			// 使用第一个可用的 API Key（已按优先级排序）
			currentAPIKey = apiKeys[0].APIKey
			currentKeyID = apiKeys[0].ID
			logger.Printf("流式请求使用平台 Claude API Key (模型代码: %s, 模型名: %s, KeyID: %d)", modelCode, model, apiKeys[0].ID)
		}
		
		// 获取可用密钥数量（用于重试）
		maxAttempts := 1 // 默认尝试一次
		if !useUserKey && modelCode != "" {
			apiKeys, _ := s.keyManager.modelManager.GetAvailableAPIKeys(modelCode)
			maxAttempts = len(apiKeys)
			if maxAttempts == 0 {
				maxAttempts = 1 // 至少尝试一次
			}
		}
		
		// 尝试所有可用的API密钥
		for attempt := 0; attempt < maxAttempts; attempt++ {
			// 如果需要重试，获取下一个可用的 API Key
			if attempt > 0 && !useUserKey && modelCode != "" {
				apiKeys, _ := s.keyManager.modelManager.GetAvailableAPIKeys(modelCode)
				if attempt < len(apiKeys) {
					currentAPIKey = apiKeys[attempt].APIKey
					currentKeyID = apiKeys[attempt].ID
					logger.Printf("流式请求重试使用 API Key (KeyID: %d)", currentKeyID)
				}
			}
			
			// 准备请求参数
			streamParams := map[string]interface{}{
				"model":       model,
				"messages":    messages,
				"max_tokens":  int(maxTokens),
				"temperature": temperature,
				"stream":      true,
			}
			
			// 只有在非思考模式下才添加top_p参数
			if thinking == nil || thinking["type"] != "enabled" {
				if topP != 0 {
					streamParams["top_p"] = topP
				}
			}
			
			// 添加系统消息（如果存在）
			if systemMessage != "" {
				streamParams["system"] = systemMessage
			}
			
			// 添加思考模式（如果启用）
			if thinking != nil {
				streamParams["thinking"] = thinking
				logger.Printf("已启用思考模式: %v", thinking)
			}
			
			// 记录完整的请求参数（不含敏感信息）
			requestLog := make(map[string]interface{})
			for k, v := range streamParams {
				if k == "messages" {
					requestLog[k] = fmt.Sprintf("[%d messages]", len(messages))
				} else {
					requestLog[k] = v
				}
			}
			requestLogJSON, _ := json.Marshal(requestLog)
			logger.Printf("完整流式请求参数: %s", string(requestLogJSON))
			
			// 将 API Key 信息传递给流式请求
			streamParams["_api_key"] = currentAPIKey
			streamParams["_key_id"] = currentKeyID
			
			// 创建流式请求
			stream, err := s.sendStreamRequest(ctx, streamParams)
			if err != nil {
				// 更新密钥失败状态
				if currentKeyID > 0 {
					s.keyManager.UpdateKeyStatus(currentKeyID, false, err.Error())
				}
				
				// 记录当前使用的API密钥
				if currentAPIKey != "" {
					keyPreview := currentAPIKey
					if len(keyPreview) > 12 {
						keyPreview = keyPreview[:8] + "..." + keyPreview[len(keyPreview)-4:]
					} else {
						keyPreview = "***"
					}
					logger.Printf("使用API密钥 %s 流式请求失败: %v", keyPreview, err)
					
					// 标记当前密钥为不可用（仅平台 Key）
					if !useUserKey {
						s.keyManager.MarkKeyUnavailable(currentAPIKey)
					}
				}
				
				// 尝试使用下一个密钥（如果还有可用的）
				if attempt < maxAttempts-1 {
					logger.Printf("尝试使用下一个API密钥 (尝试 %d/%d)", attempt+1, maxAttempts)
					// 下一次循环会自动获取下一个 Key（已在循环开始处理）
					continue
				} else {
					// 所有密钥都已尝试过
					logger.Print("所有API密钥都已尝试，但请求仍然失败")
					outputChan <- createErrorChunk("所有API密钥都已尝试，但请求仍然失败")
					return
				}
			}
			
			// 更新密钥成功状态
			if currentKeyID > 0 {
				s.keyManager.UpdateKeyStatus(currentKeyID, true, "")
			}
			
			logger.Print("成功创建Claude Messages API流式请求")
			
			// 处理流式响应
			for chunk := range stream {
				outputChan <- chunk
			}
			
			return
		}
		
		// 如果所有密钥都尝试过仍然失败
		outputChan <- createErrorChunk("所有API密钥都已尝试，但请求仍然失败")
	}()
	
	return outputChan
}

// sendRequest 发送请求到Claude API
func (s *ClaudeService) sendRequest(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	// 构建请求URL
	apiURL := s.keyManager.GetBaseURL() + "/v1/messages"
	
	// 获取 API Key（优先使用从外层传递的 Key）
	var apiKey string
	var keyID int
	
	// 检查是否从外层传递了 API Key
	if passedKey, ok := params["_api_key"].(string); ok && passedKey != "" {
		apiKey = passedKey
		if passedKeyID, ok := params["_key_id"].(int); ok {
			keyID = passedKeyID
		}
		logger.Printf("使用传递的 API Key (模型: %s, KeyID: %d)", params["model"], keyID)
	} else if s.currentAPIKey != "" {
		// 使用已设置的 API Key（从 CreateChatCompletionNonStream 中设置的）
		apiKey = s.currentAPIKey
		keyID = s.currentKeyID
		logger.Printf("使用已设置的 API Key (模型: %s, KeyID: %d)", params["model"], keyID)
	} else {
		// 降级方案：检查是否使用用户自己的 Claude Key
		useUserKey, _ := params["_use_user_key"].(bool)
		userClaudeKey, _ := params["_user_claude_key"].(string)
		
		if useUserKey && userClaudeKey != "" {
			// 使用用户自己的 Claude Key
			apiKey = userClaudeKey
			keyID = 0
			logger.Printf("使用用户自己的 Claude Key (模型: %s)", params["model"])
		} else {
			// 使用平台的 Claude Key（降级到默认方法）
			var err error
			apiKey, keyID, err = s.keyManager.GetKeyWithID()
			if err != nil {
				logger.Printf("获取平台 Claude API 密钥失败: %v", err)
				return map[string]interface{}{
					"error": map[string]interface{}{
						"message": fmt.Sprintf("无法获取API密钥: %v", err),
						"type":    "internal_error",
					},
				}, err
			}
			logger.Printf("使用平台 Claude API Key (模型: %s, KeyID: %d)", params["model"], keyID)
		}
	}
	
	// 清理内部标记，不发送给 Claude API
	delete(params, "_use_user_key")
	delete(params, "_user_claude_key")
	delete(params, "api_key")
	delete(params, "model_code")
	delete(params, "_api_key")
	delete(params, "_key_id")
	delete(params, "_system_prompt") // 清理内部标记
	
	// 序列化请求参数为JSON
	jsonData, err := json.Marshal(params)
	if err != nil {
		logger.Printf("序列化请求参数失败: %v", err)
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("序列化请求失败: %v", err),
				"type":    "internal_error",
			},
		}, err
	}
	
	logger.Printf("发送请求到 Claude API: %s, 参数长度: %d", apiURL, len(jsonData))
	
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		logger.Printf("创建HTTP请求失败: %v", err)
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("创建请求失败: %v", err),
				"type":    "internal_error",
			},
		}, err
	}
	
	// 设置请求头（Claude API 要求）
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	// 添加 Files API beta header（如果消息中包含文件引用）
	if hasFileReference(params) {
		req.Header.Set("anthropic-beta", "files-api-2025-04-14")
	}
	
	// 创建 HTTP 客户端（设置超时）
	httpClient := &http.Client{Timeout: 300 * time.Second} // 增加到5分钟，适应长时间任务（如安全扫描）
	
	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Printf("发送HTTP请求失败: %v", err)
		// 更新密钥状态为失败
		if keyID > 0 {
			s.keyManager.UpdateKeyStatus(keyID, false, fmt.Sprintf("请求失败: %v", err))
		}
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("请求失败: %v", err),
				"type":    "api_error",
			},
		}, err
	}
	defer resp.Body.Close()
	
	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("读取响应失败: %v", err)
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("读取响应失败: %v", err),
				"type":    "internal_error",
			},
		}, err
	}
	
	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Printf("Claude API 返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
		// 更新密钥状态为失败
		if keyID > 0 {
			s.keyManager.UpdateKeyStatus(keyID, false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
		}
		
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return errorResp, fmt.Errorf("API error: %d", resp.StatusCode)
		}
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("API错误 (状态码 %d): %s", resp.StatusCode, string(body)),
				"type":    "api_error",
			},
		}, fmt.Errorf("API error: %d", resp.StatusCode)
	}
	
	// 解析响应JSON
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		logger.Printf("解析响应JSON失败: %v, 响应内容: %s", err, string(body))
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("解析响应失败: %v", err),
				"type":    "internal_error",
			},
		}, err
	}
	
	// 检查响应中是否有错误
	if errorData, hasError := response["error"]; hasError {
		logger.Printf("Claude API 返回错误: %v", errorData)
		// 更新密钥状态为失败
		if keyID > 0 {
			s.keyManager.UpdateKeyStatus(keyID, false, fmt.Sprintf("API错误: %v", errorData))
		}
		return response, fmt.Errorf("API error: %v", errorData)
	}
	
	// 请求成功，更新密钥状态
	if keyID > 0 {
		s.keyManager.UpdateKeyStatus(keyID, true, "")
	}
	
	logger.Printf("Claude API 请求成功，响应长度: %d", len(body))
	return response, nil
}

// sendStreamRequest 发送流式请求到Claude API
func (s *ClaudeService) sendStreamRequest(ctx context.Context, params map[string]interface{}) (chan string, error) {
	streamChan := make(chan string)
	
	go func() {
		defer close(streamChan)
		
		// 构建请求URL
		apiURL := s.keyManager.GetBaseURL() + "/v1/messages"
		
		// 获取 API Key（优先使用从外层传递的 Key）
		var apiKey string
		var keyID int
		
		// 检查是否从外层传递了 API Key
		if passedKey, ok := params["_api_key"].(string); ok && passedKey != "" {
			apiKey = passedKey
			if passedKeyID, ok := params["_key_id"].(int); ok {
				keyID = passedKeyID
			}
			logger.Printf("使用传递的 API Key (模型: %s, KeyID: %d, 流式)", params["model"], keyID)
		} else {
			// 降级方案：检查是否使用用户自己的 Claude Key
			useUserKey, _ := params["_use_user_key"].(bool)
			userClaudeKey, _ := params["_user_claude_key"].(string)
			
			if useUserKey && userClaudeKey != "" {
				// 使用用户自己的 Claude Key
				apiKey = userClaudeKey
				keyID = 0
				logger.Printf("使用用户自己的 Claude Key (模型: %s, 流式)", params["model"])
			} else {
				// 使用平台的 Claude Key（降级到默认方法）
				var err error
				apiKey, keyID, err = s.keyManager.GetKeyWithID()
				if err != nil {
					logger.Printf("获取平台 Claude API 密钥失败: %v", err)
					streamChan <- createErrorChunk(fmt.Sprintf("无法获取API密钥: %v", err))
					return
				}
				logger.Printf("使用平台 Claude API Key (模型: %s, KeyID: %d, 流式)", params["model"], keyID)
			}
		}
		
		// 清理内部标记，不发送给 Claude API
		delete(params, "_use_user_key")
		delete(params, "_user_claude_key")
		delete(params, "api_key")
		delete(params, "model_code")
		delete(params, "_api_key")
		delete(params, "_key_id")
		delete(params, "_system_prompt") // 清理内部标记
		
		// 确保 stream 参数为 true
		params["stream"] = true
		
		// 序列化请求参数为JSON
		jsonData, err := json.Marshal(params)
		if err != nil {
			logger.Printf("序列化流式请求参数失败: %v", err)
			streamChan <- createErrorChunk(fmt.Sprintf("序列化请求失败: %v", err))
			return
		}
		
		logger.Printf("发送流式请求到 Claude API: %s, 参数长度: %d", apiURL, len(jsonData))
		
		// 创建HTTP请求
		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
		if err != nil {
			logger.Printf("创建流式HTTP请求失败: %v", err)
			streamChan <- createErrorChunk(fmt.Sprintf("创建请求失败: %v", err))
			return
		}
		
		// 设置请求头（Claude API 流式要求）
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Connection", "keep-alive")
		// 添加 Files API beta header（如果消息中包含文件引用）
		if hasFileReference(params) {
			req.Header.Set("anthropic-beta", "files-api-2025-04-14")
		}
		
		// 创建 HTTP 客户端（设置超时）
		httpClient := &http.Client{Timeout: 300 * time.Second} // 增加到5分钟，适应长时间任务（如安全扫描）
		
		// 发送请求
		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Printf("发送流式HTTP请求失败: %v", err)
			// 更新密钥状态为失败
			if keyID > 0 {
				s.keyManager.UpdateKeyStatus(keyID, false, fmt.Sprintf("请求失败: %v", err))
			}
			streamChan <- createErrorChunk(fmt.Sprintf("请求失败: %v", err))
			return
		}
		defer resp.Body.Close()
		
		// 检查HTTP状态码
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			logger.Printf("流式 Claude API 返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
			// 更新密钥状态为失败
			if keyID > 0 {
				s.keyManager.UpdateKeyStatus(keyID, false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
			}
			streamChan <- createErrorChunk(fmt.Sprintf("API错误 (状态码 %d): %s", resp.StatusCode, string(body)))
			return
		}
		
		// 读取SSE流式响应
		reader := bufio.NewReader(resp.Body)
		hasError := false
		
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					logger.Printf("读取流式响应失败: %v", err)
					hasError = true
				}
				break
			}
			
			// 去除行尾换行符
			line = strings.TrimSpace(line)
			
			// 跳过空行
			if line == "" {
				continue
			}
			
			// 处理 SSE 数据行 (格式: "data: {...}")
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				
				// 检查是否是结束标记
				if data == "[DONE]" {
					break
				}
				
				// 转发 Claude 的流式事件 JSON
				streamChan <- data
			}
		}
		
		// 请求完成，更新密钥状态
		if keyID > 0 {
			if hasError {
				s.keyManager.UpdateKeyStatus(keyID, false, "流式响应读取失败")
			} else {
				s.keyManager.UpdateKeyStatus(keyID, true, "")
			}
		}
		
		logger.Printf("Claude API 流式请求完成")
	}()
	
	return streamChan, nil
}

// createErrorChunk 创建错误响应块
func createErrorChunk(message string) string {
	errorChunk := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "server_error",
			"code":    "internal_error",
		},
	}
	chunkJSON, _ := json.Marshal(errorChunk)
	return string(chunkJSON)
}

// random 函数用于模拟随机数生成
func random(min, max int) int {
	return min + rand.Intn(max-min)
}

// normalizeBaseURL 规范化 baseURL，去除末尾的 /v1 或 /
func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	// 如果 baseURL 以 /v1 结尾，去除它
	if strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}
	return baseURL
}

// GetModels 获取模型列表（Claude API）
// modelsEndpoint 可选：模型列表端点，Claude 通常不需要，留作兼容
func (s *ClaudeService) GetModels(ctx context.Context, baseURL string, apiKey string, modelsEndpoint ...string) (map[string]interface{}, error) {
	// Claude API 不提供模型列表接口，返回固定列表
	// 或者可以尝试调用其他端点
	// 规范化 baseURL，避免重复的 /v1
	normalizedBaseURL := baseURL
	if normalizedBaseURL == "" {
		normalizedBaseURL = s.keyManager.GetBaseURL()
	}
	normalizedBaseURL = normalizeBaseURL(normalizedBaseURL)
	
	// 构建请求URL（如果提供了自定义 modelsEndpoint 则使用之）
	path := "/v1/models"
	if len(modelsEndpoint) > 0 && modelsEndpoint[0] != "" {
		path = modelsEndpoint[0]
	}
	apiURL := normalizedBaseURL + path
	
	logger.Printf("获取Claude模型列表: %s", apiURL)
	
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		logger.Printf("创建HTTP请求失败: %v", err)
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	} else {
		// 如果没有提供 API Key，尝试使用默认的
		if s.currentAPIKey != "" {
			req.Header.Set("x-api-key", s.currentAPIKey)
		} else {
			key, _, err := s.keyManager.GetKeyWithID()
			if err == nil {
				req.Header.Set("x-api-key", key)
			}
		}
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	
	// 创建 HTTP 客户端
	httpClient := &http.Client{Timeout: 30 * time.Second}
	
	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Printf("发送HTTP请求失败: %v", err)
		// Claude API 可能不支持 /v1/models 端点，返回空列表
		return map[string]interface{}{
			"object": "list",
			"data":   []interface{}{},
		}, nil
	}
	defer resp.Body.Close()
	
	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("读取响应失败: %v", err)
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}
	
	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Printf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
		// Claude API 可能不支持 /v1/models 端点，返回空列表
		return map[string]interface{}{
			"object": "list",
			"data":   []interface{}{},
		}, nil
	}
	
	// 解析响应JSON
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		logger.Printf("解析响应JSON失败: %v, 响应内容: %s", err, string(body))
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	
	logger.Printf("成功获取Claude模型列表")
	return response, nil
}

// hasFileReference 检查请求参数中是否包含文件引用（使用 file_id）
func hasFileReference(params map[string]interface{}) bool {
	messages, ok := params["messages"].([]interface{})
	if !ok {
		return false
	}
	
	// 遍历消息列表，检查是否有文件引用
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		
		content := msgMap["content"]
		switch contentVal := content.(type) {
		case []interface{}:
			// 数组格式的 content
			for _, part := range contentVal {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					continue
				}
				
				partType, _ := partMap["type"].(string)
				if partType == "document" || partType == "image" {
					if source, ok := partMap["source"].(map[string]interface{}); ok {
						if sourceType, _ := source["type"].(string); sourceType == "file" {
							if _, hasFileID := source["file_id"]; hasFileID {
								return true
							}
						}
					}
				}
			}
		}
	}
	
	return false
}

// UploadFile 上传文件到 Claude Files API 并返回 file_id
// fileBytes: 文件字节内容
// filename: 文件名
// mimeType: MIME 类型
// apiKey: API 密钥（可选，如果不提供则使用当前密钥）
// 返回 file_id 和错误信息
func (s *ClaudeService) UploadFile(ctx context.Context, fileBytes []byte, filename string, mimeType string, apiKey string) (string, error) {
	// 构建上传 URL
	uploadURL := s.keyManager.GetBaseURL() + "/v1/files"
	
	// 如果没有提供 API Key，使用当前密钥
	if apiKey == "" {
		if s.currentAPIKey != "" {
			apiKey = s.currentAPIKey
		} else {
			var err error
			apiKey, _, err = s.keyManager.GetKeyWithID()
			if err != nil {
				return "", fmt.Errorf("无法获取API密钥: %v", err)
			}
		}
	}
	
	// 创建 multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	
	// 添加文件字段
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("创建表单文件字段失败: %v", err)
	}
	
	_, err = fileWriter.Write(fileBytes)
	if err != nil {
		return "", fmt.Errorf("写入文件数据失败: %v", err)
	}
	
	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("关闭表单写入器失败: %v", err)
	}
	
	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &requestBody)
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}
	
	// 设置请求头
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "files-api-2025-04-14")
	
	// 创建 HTTP 客户端
	httpClient := &http.Client{Timeout: 300 * time.Second}
	
	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %v", err)
	}
	defer resp.Body.Close()
	
	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}
	
	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		logger.Printf("文件上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("文件上传失败 (状态码 %d): %s", resp.StatusCode, string(body))
	}
	
	// 解析响应 JSON
	var uploadResponse map[string]interface{}
	if err := json.Unmarshal(body, &uploadResponse); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	
	// 提取 file_id
	fileID, ok := uploadResponse["id"].(string)
	if !ok {
		return "", fmt.Errorf("响应中缺少 file_id 字段")
	}
	
	logger.Printf("文件上传成功: %s (file_id: %s, size: %d bytes)", filename, fileID, len(fileBytes))
	return fileID, nil
} 