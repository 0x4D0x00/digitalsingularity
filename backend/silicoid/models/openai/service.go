// OpenAI API服务：处理OpenAI模型的请求

package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	pathconfig "digitalsingularity/backend/common/configs"
	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/silicoid/models/manager"
	"gopkg.in/ini.v1"
)

// 获取logger
var logger = log.New(log.Writer(), "openai_service: ", log.LstdFlags)

// OpenAIService OpenAI服务
type OpenAIService struct {
	readWrite       *datahandle.CommonReadWriteService
	modelManager    *manager.ModelManager
	usingNewSDK     bool
	defaultModel    string
	apiKey          string
	baseURL         string
	endpoint        string
	httpClient      *http.Client
}

// NewOpenAIService 创建OpenAI服务实例
func NewOpenAIService() *OpenAIService {
	readWrite, _ := datahandle.NewCommonReadWriteService("database")

	// 加载配置
	config := loadConfig()

	service := &OpenAIService{
		readWrite:    readWrite,
		modelManager: manager.NewModelManager(),
		usingNewSDK:  true,
		defaultModel: config["model"],
		apiKey:       config["api_key"],
		baseURL:      config["api_base"],
		endpoint:     config["endpoint"],
		httpClient:   &http.Client{Timeout: 300 * time.Second}, // 增加到5分钟，适应长时间任务（如安全扫描）
	}
	
	logger.Printf("初始化OpenAI服务完成 - 模型: %s, API基础URL: %s", service.defaultModel, service.baseURL)
	return service
}

// loadConfig 加载 OpenAI 配置
func loadConfig() map[string]string {
	// 使用统一的路径配置
	pathCfg := pathconfig.GetInstance()
	configPath := pathCfg.GetConfigPath("config.ini")
	
	logger.Printf("尝试从路径读取OpenAI配置: %s", configPath)
	
	// 默认配置
	openaiConfig := map[string]string{
		"api_key":  "",
		"api_base": "https://api.openai.com",
		"endpoint": "/v1/chat/completions",
		"model":    "gpt-3.5-turbo",
	}
	
	// 尝试从 config.ini 读取配置
	cfg, err := ini.Load(configPath)
	if err != nil {
		logger.Printf("无法加载配置文件 %s: %v，使用默认配置", configPath, err)
		return openaiConfig
	}
	
	// 读取 OpenAI 配置段
	section, err := cfg.GetSection("OpenAI")
	if err != nil {
		logger.Printf("配置文件中没有 [OpenAI] 段，使用默认配置")
		return openaiConfig
	}
	
	// 读取配置项
	if section.HasKey("apikey") {
		openaiConfig["api_key"] = section.Key("apikey").String()
	}
	if section.HasKey("baseurl") {
		openaiConfig["api_base"] = section.Key("baseurl").String()
	}
	if section.HasKey("endpoint") {
		openaiConfig["endpoint"] = section.Key("endpoint").String()
	}
	if section.HasKey("model") {
		openaiConfig["model"] = section.Key("model").String()
	}
	
	// 验证必需配置
	if openaiConfig["api_key"] == "" {
		logger.Printf("警告: 未配置 OpenAI API 密钥")
	}
	
	logger.Printf("加载OpenAI配置成功 - 模型: %s, API地址: %s%s", 
		openaiConfig["model"], openaiConfig["api_base"], openaiConfig["endpoint"])
	
	return openaiConfig
}

// CreateChatCompletionNonStream 创建聊天完成请求
func (s *OpenAIService) CreateChatCompletionNonStream(ctx context.Context, data map[string]interface{}) map[string]interface{} {
	model, _ := data["model"].(string)
	if model == "" {
		model = s.defaultModel
	}

	logger.Printf("发送OpenAI聊天完成请求，模型: %s", model)

	// 记录请求开始时间
	startTime := time.Now()

	// 发送请求到OpenAI API
	response := s.sendRequest(ctx, data)

	// 记录请求时间
	elapsedTime := time.Since(startTime)
	logger.Printf("OpenAI响应时间: %.2f秒", elapsedTime.Seconds())

	// 记录使用的token
	usage, _ := response["usage"].(map[string]interface{})
	if usage != nil {
		promptTokens, _ := usage["prompt_tokens"].(float64)
		completionTokens, _ := usage["completion_tokens"].(float64)
		totalTokens, _ := usage["total_tokens"].(float64)

		logger.Printf("Token使用: 提示=%.0f，完成=%.0f，总计=%.0f",
			promptTokens, completionTokens, totalTokens)
	}

	return response
}

// CreateChatCompletionStream 创建流式聊天完成请求
func (s *OpenAIService) CreateChatCompletionStream(ctx context.Context, data map[string]interface{}) chan string {
	outputChan := make(chan string)
	
	go func() {
		defer close(outputChan)
		
		model, _ := data["model"].(string)
		if model == "" {
			model = s.defaultModel
		}
		
	logger.Printf("发送OpenAI流式聊天完成请求，模型: %s", model)
	
	// 确保stream参数设置为True
	data["stream"] = true
	
	// 创建流式请求到OpenAI API
	stream := s.sendStreamRequest(ctx, data)
	
	// 直接转发每个块（已经是 SSE 格式：data: {...}\n\n）
	for chunk := range stream {
		outputChan <- chunk
	}
	
	// 发送结束标记
	outputChan <- "data: [DONE]\n\n"
	}()
	
	return outputChan
}

// sendRequest 发送请求到OpenAI API
func (s *OpenAIService) sendRequest(ctx context.Context, params map[string]interface{}) map[string]interface{} {
	// 获取模型名称
	model, _ := params["model"].(string)
	if model == "" {
		model = s.defaultModel
	}
	
	// 处理 API Key：根据拦截器传来的标记决定使用哪个 Key
	var apiKey string
	
	// 检查是否使用用户自己的 OpenAI Key（拦截器设置的标记）
	useUserKey, _ := params["_use_user_key"].(bool)
	userOpenAIKey, _ := params["_user_openai_key"].(string)
	
	// 获取 model_code 用于查询 API 密钥
	modelCode, _ := params["model_code"].(string)
	logger.Printf("DEBUG: 从参数中获取的 model_code: '%s'", modelCode)
	if modelCode == "" {
		modelCode = "OpenAI" // 默认使用 OpenAI
		logger.Printf("DEBUG: model_code 为空，使用默认值: %s", modelCode)
	}
	
	// 获取 base_url 和 endpoint（优先使用从数据库获取的配置）
	baseURL, _ := params["_base_url"].(string)
	if baseURL == "" {
		baseURL = s.baseURL // 如果没有，使用默认值
		logger.Printf("DEBUG: base_url 为空，使用默认值: %s", baseURL)
	} else {
		logger.Printf("DEBUG: 使用从数据库获取的 base_url: %s", baseURL)
	}
	
	endpoint, _ := params["_endpoint"].(string)
	if endpoint == "" {
		endpoint = s.endpoint // 如果没有，使用默认值
		logger.Printf("DEBUG: endpoint 为空，使用默认值: %s", endpoint)
	} else {
		logger.Printf("DEBUG: 使用从数据库获取的 endpoint: %s", endpoint)
	}
	
	// 构建请求URL
	apiURL := baseURL + endpoint
	
	// 清理内部标记，不发送给 OpenAI
	delete(params, "_use_user_key")
	delete(params, "_user_openai_key")
	delete(params, "api_key")
	delete(params, "model_code")
	delete(params, "_base_url")
	delete(params, "_endpoint")
	delete(params, "role_name")
	
	switch {
	case useUserKey && userOpenAIKey != "":
		// 场景1: 使用用户自己的 OpenAI Key
		apiKey = userOpenAIKey
		logger.Printf("使用用户自己的 OpenAI Key (模型: %s)", model)
		
	default:
		// 场景2: 使用平台的 OpenAI Key
		apiKeys, err := s.modelManager.GetAvailableAPIKeys(modelCode)
		if err != nil || len(apiKeys) == 0 {
			logger.Printf("获取平台API密钥失败 (模型代码: %s, 模型名: %s): %v", modelCode, model, err)
			return map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("无法获取API密钥: %v", err),
					"type":    "internal_error",
				},
			}
		}
		
		apiKey = apiKeys[0].APIKey
		// 显示 API Key 的前几个字符和长度用于调试（不显示完整密钥）
		keyPreview := apiKey
		if len(keyPreview) > 10 {
			keyPreview = keyPreview[:10] + "..."
		}
		logger.Printf("使用平台API Key (模型代码: %s, 模型名: %s, KeyID: %d, Key预览: %s, 长度: %d)", 
			modelCode, model, apiKeys[0].ID, keyPreview, len(apiKey))
	}
	
	// 检查并记录工具信息（用于验证工具传递）
	if tools, ok := params["tools"].([]interface{}); ok && len(tools) > 0 {
		logger.Printf("📤 发送请求包含 %d 个工具到模型 %s", len(tools), model)
		// 记录前3个工具名称作为示例（避免日志过长）
		for i, tool := range tools {
			if i >= 3 {
				break
			}
			if toolMap, ok := tool.(map[string]interface{}); ok {
				if funcData, ok := toolMap["function"].(map[string]interface{}); ok {
					if name, ok := funcData["name"].(string); ok {
						logger.Printf("  工具 %d: %s", i+1, name)
					}
				}
			}
		}
		if len(tools) > 3 {
			logger.Printf("  ... 还有 %d 个其他工具", len(tools)-3)
		}
	}

	// 序列化请求参数为JSON
	jsonData, err := json.Marshal(params)
	if err != nil {
		logger.Printf("序列化请求参数失败: %v", err)
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("序列化请求失败: %v", err),
				"type":    "internal_error",
			},
		}
	}
	
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		logger.Printf("创建HTTP请求失败: %v", err)
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("创建请求失败: %v", err),
				"type":    "internal_error",
			},
		}
	}
	
	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	
	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.Printf("发送HTTP请求失败: %v", err)
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("请求失败: %v", err),
				"type":    "api_error",
			},
		}
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
		}
	}
	
	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		logger.Printf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return errorResp
		}
		return map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("API错误 (状态码 %d): %s", resp.StatusCode, string(body)),
				"type":    "api_error",
			},
		}
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
		}
	}
	
	return response
}

// sendStreamRequest 发送流式请求到OpenAI API
func (s *OpenAIService) sendStreamRequest(ctx context.Context, params map[string]interface{}) chan string {
	streamChan := make(chan string)
	
	go func() {
		defer close(streamChan)
		
		// 获取模型名称
		model, _ := params["model"].(string)
		if model == "" {
			model = s.defaultModel
		}
		
		// 处理 API Key：根据拦截器传来的标记决定使用哪个 Key
		var apiKey string
		
		// 检查是否使用用户自己的 OpenAI Key（拦截器设置的标记）
		useUserKey, _ := params["_use_user_key"].(bool)
		userOpenAIKey, _ := params["_user_openai_key"].(string)
	
		// 获取 model_code 用于查询 API 密钥
		modelCode, _ := params["model_code"].(string)
		logger.Printf("DEBUG: 流式请求从参数中获取的 model_code: '%s'", modelCode)
		if modelCode == "" {
			modelCode = "OpenAI" // 默认使用 OpenAI
			logger.Printf("DEBUG: 流式请求 model_code 为空，使用默认值: %s", modelCode)
		}
		
		// 获取 base_url 和 endpoint（优先使用从数据库获取的配置）
		baseURL, _ := params["_base_url"].(string)
		if baseURL == "" {
			baseURL = s.baseURL // 如果没有，使用默认值
			logger.Printf("DEBUG: 流式请求 base_url 为空，使用默认值: %s", baseURL)
		} else {
			logger.Printf("DEBUG: 流式请求使用从数据库获取的 base_url: %s", baseURL)
		}
		
		endpoint, _ := params["_endpoint"].(string)
		if endpoint == "" {
			endpoint = s.endpoint // 如果没有，使用默认值
			logger.Printf("DEBUG: 流式请求 endpoint 为空，使用默认值: %s", endpoint)
		} else {
			logger.Printf("DEBUG: 流式请求使用从数据库获取的 endpoint: %s", endpoint)
		}
		
		// 构建请求URL
		apiURL := baseURL + endpoint
		
		// 清理内部标记，不发送给 OpenAI
		delete(params, "_use_user_key")
		delete(params, "_user_openai_key")
		delete(params, "api_key")
		delete(params, "model_code")
		delete(params, "_base_url")
		delete(params, "_endpoint")
		delete(params, "role_name")
		
		switch {
		case useUserKey && userOpenAIKey != "":
			// 场景1: 使用用户自己的 OpenAI Key
			apiKey = userOpenAIKey
			logger.Printf("使用用户自己的 OpenAI Key (模型: %s, 流式)", model)
			
		default:
			// 场景2: 使用平台的 OpenAI Key
			apiKeys, err := s.modelManager.GetAvailableAPIKeys(modelCode)
			if err != nil || len(apiKeys) == 0 {
				logger.Printf("获取平台API密钥失败 (模型代码: %s, 模型名: %s, 流式): %v", modelCode, model, err)
				errorData := map[string]interface{}{
					"error": map[string]interface{}{
						"message": fmt.Sprintf("无法获取API密钥: %v", err),
						"type":    "internal_error",
					},
				}
				errorJSON, _ := json.Marshal(errorData)
				streamChan <- fmt.Sprintf("data: %s\n\n", string(errorJSON))
				return
			}
			
			apiKey = apiKeys[0].APIKey
			logger.Printf("使用平台API Key (模型代码: %s, 模型名: %s, KeyID: %d, 流式)", modelCode, model, apiKeys[0].ID)
		}
		
		// 检查并记录工具信息（用于验证工具传递）
		if tools, ok := params["tools"].([]interface{}); ok && len(tools) > 0 {
			logger.Printf("📤 发送流式请求包含 %d 个工具到模型 %s", len(tools), model)
			// 记录前3个工具名称作为示例（避免日志过长）
			for i, tool := range tools {
				if i >= 3 {
					break
				}
				if toolMap, ok := tool.(map[string]interface{}); ok {
					if funcData, ok := toolMap["function"].(map[string]interface{}); ok {
						if name, ok := funcData["name"].(string); ok {
							logger.Printf("  工具 %d: %s", i+1, name)
						}
					}
				}
			}
			if len(tools) > 3 {
				logger.Printf("  ... 还有 %d 个其他工具", len(tools)-3)
			}
		}

		// 序列化请求参数为JSON
		jsonData, err := json.Marshal(params)
		if err != nil {
			logger.Printf("序列化流式请求参数失败: %v", err)
			errorData := map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("序列化请求失败: %v", err),
					"type":    "internal_error",
				},
			}
			errorJSON, _ := json.Marshal(errorData)
			streamChan <- fmt.Sprintf("data: %s\n\n", string(errorJSON))
			return
		}
		
		// 创建HTTP请求
		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
		if err != nil {
			logger.Printf("创建流式HTTP请求失败: %v", err)
			errorData := map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("创建请求失败: %v", err),
					"type":    "internal_error",
				},
			}
			errorJSON, _ := json.Marshal(errorData)
			streamChan <- fmt.Sprintf("data: %s\n\n", string(errorJSON))
			return
		}
		
		// 设置请求头
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Connection", "keep-alive")
		
		// 发送请求
		resp, err := s.httpClient.Do(req)
		if err != nil {
			logger.Printf("发送流式HTTP请求失败: %v", err)
			errorData := map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("请求失败: %v", err),
					"type":    "api_error",
				},
			}
			errorJSON, _ := json.Marshal(errorData)
			streamChan <- fmt.Sprintf("data: %s\n\n", string(errorJSON))
			return
		}
		defer resp.Body.Close()
		
		// 检查HTTP状态码
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			logger.Printf("流式API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
			errorData := map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("API错误 (状态码 %d): %s", resp.StatusCode, string(body)),
					"type":    "api_error",
				},
			}
			errorJSON, _ := json.Marshal(errorData)
			streamChan <- fmt.Sprintf("data: %s\n\n", string(errorJSON))
			return
		}
		
		// 读取SSE流式响应
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					logger.Printf("读取流式响应失败: %v", err)
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
				
		// 转发完整的 SSE 消息格式
		streamChan <- fmt.Sprintf("data: %s\n\n", data)
		}
	}
}()

return streamChan
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

// GetModels 获取模型列表（OpenAI 兼容 API）
// modelsEndpoint 可选：模型列表端点(如 /v1/models, /api/v1/models)，为空则默认 /v1/models
func (s *OpenAIService) GetModels(ctx context.Context, baseURL string, apiKey string, modelsEndpoint ...string) (map[string]interface{}, error) {
	// 规范化 baseURL，避免重复的 /v1
	normalizedBaseURL := baseURL
	if normalizedBaseURL == "" {
		normalizedBaseURL = s.baseURL
	}
	normalizedBaseURL = normalizeBaseURL(normalizedBaseURL)

	// 确定模型列表端点
	modelsPath := "/v1/models"
	if len(modelsEndpoint) > 0 && modelsEndpoint[0] != "" {
		modelsPath = modelsEndpoint[0]
	}

	// 构建请求URL
	apiURL := normalizedBaseURL + modelsPath

	logger.Printf("获取模型列表: %s", apiURL)
	
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		logger.Printf("创建HTTP请求失败: %v", err)
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	
	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		// 如果没有提供 API Key，尝试使用默认的
		if s.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.apiKey)
		}
	}
	
	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.Printf("发送HTTP请求失败: %v", err)
		return nil, fmt.Errorf("请求失败: %v", err)
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
		return nil, fmt.Errorf("API错误 (状态码 %d): %s", resp.StatusCode, string(body))
	}
	
	// 解析响应JSON
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		logger.Printf("解析响应JSON失败: %v, 响应内容: %s", err, string(body))
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}
	
	logger.Printf("成功获取模型列表，共 %d 个模型", len(response))
	return response, nil
}