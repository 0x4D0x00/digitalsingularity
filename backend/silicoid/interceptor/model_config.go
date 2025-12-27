package interceptor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"digitalsingularity/backend/silicoid/models/manager"
)

// GetModelConfig 获取模型配置
// 支持通过 model_code 或 model_name 查询
func (s *SilicoIDInterceptor) GetModelConfig(modelCodeOrName string) (*manager.ModelConfig, error) {
	return s.modelManager.GetModelConfig(modelCodeOrName)
}

// GetAllModels 获取所有模型配置
func (s *SilicoIDInterceptor) GetAllModels() ([]*manager.ModelConfig, error) {
	return s.modelManager.GetAllModels()
}

// GetAvailableAPIKeys 获取模型的可用 API Keys
func (s *SilicoIDInterceptor) GetAvailableAPIKeys(modelCode string) ([]*manager.APIKeyConfig, error) {
	return s.modelManager.GetAvailableAPIKeys(modelCode)
}


// HandleModels 处理获取模型列表的请求
func (s *SilicoIDInterceptor) HandleModels(c *gin.Context) {
	requestID := fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))
	logger.Printf("[%s] 收到获取模型列表请求", requestID)
	
	// 获取模型列表不需要API密钥验证，这是公开接口
	
	// 使用 modelManager 获取所有公司的所有可用模型 (从各公司模型表读取)
	models := make([]map[string]interface{}, 0)
	now := int(time.Now().Unix())
	
	// 从各公司模型表中获取所有可用模型
	providerModels, err := s.modelManager.GetAllProviderModels()
	if err != nil {
		logger.Printf("[%s] ⚠️  从各公司模型表加载模型失败: %v，尝试从已配置模型加载", requestID, err)
		
		// 降级方案：如果从各公司模型表加载失败，回退到已配置模型列表
		modelConfigs, err2 := s.modelManager.GetAllModels()
		if err2 != nil {
			logger.Printf("[%s] ⚠️  从已配置模型加载也失败: %v，返回空列表", requestID, err2)
		} else {
			for _, modelConfig := range modelConfigs {
				modelID := modelConfig.ModelName
				if modelID == "" {
					modelID = modelConfig.ModelCode
				}
				
				provider := modelConfig.Provider
				if provider == "" {
					provider = "unknown"
				}
				
				// 为 DigitalSingularity 提供商的模型提供固定的对外展示 ID
				if provider == "DigitalSingularity" {
					modelID = "PotAGI"
				}
				
				models = append(models, map[string]interface{}{
					"id":       modelID,
					"object":   "model",
					"created":  now,
					"owned_by": provider,
				})
			}
			logger.Printf("[%s] ✅ 从已配置模型加载 %d 个模型", requestID, len(models))
		}
	} else {
		// 使用各公司模型表中的所有可用模型
		for _, pm := range providerModels {
			modelID := ""
			if id, ok := pm["id"].(string); ok {
				modelID = id
			} else if modelName, ok := pm["model_name"].(string); ok {
				modelID = modelName
			}
			
			if modelID == "" {
				continue
			}
			
			provider := ""
			if p, ok := pm["provider"].(string); ok {
				provider = p
			} else {
				provider = "unknown"
			}

			// 为 DigitalSingularity 提供商的模型提供固定的对外展示 ID，
			// 这样前端和客户端看到的就是一个稳定友好的模型名称，
			// 而不会暴露真实的后端模型路径或内部标识。
			if provider == "DigitalSingularity" {
				// 如果后续有多个内部模型，可以在这里根据 modelID 再做细分映射。
				modelID = "PotAGI"
			}
			
			models = append(models, map[string]interface{}{
				"id":       modelID,
				"object":   "model",
				"created":  now,
				"owned_by": provider,
			})
		}
		logger.Printf("[%s] ✅ 从各公司模型表加载 %d 个模型", requestID, len(models))
	}
	
	logger.Printf("[%s] 返回 %d 个模型", requestID, len(models))
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

// GetModelsFromAPI 通过 interceptor 调用各个服务的 GetModels 方法获取模型列表
// provider: 提供商名称（如 "Anthropic", "OpenAI", "DeepSeek" 等）
// baseURL: API 基础 URL（如果为空，将从数据库获取）
// apiKey: API 密钥（如果为空，将从平台获取）
func (s *SilicoIDInterceptor) GetModelsFromAPI(ctx context.Context, provider string, baseURL string, apiKey string) (map[string]interface{}, error) {
	logger.Printf("通过 interceptor 获取 %s 模型列表 (baseURL: %s)", provider, baseURL)
	
	// 如果 apiKey 为空，尝试从平台获取
	if apiKey == "" {
		// 根据 provider 确定 model_code（通常 provider 和 model_code 相同或相似）
		modelCode := provider
		
		// 特殊处理：Anthropic 对应 Claude
		if provider == "Anthropic" {
			modelCode = "Claude"
		}
		
		logger.Printf("API Key 为空，尝试从平台获取 (provider: %s, model_code: %s)", provider, modelCode)
		
		// 尝试获取模型配置
		modelConfig, err := s.modelManager.GetModelConfig(modelCode)
		if err == nil && modelConfig != nil {
			// 如果 baseURL 也为空，使用模型配置中的 baseURL
			if baseURL == "" {
				baseURL = modelConfig.BaseURL
				logger.Printf("从模型配置获取 baseURL: %s", baseURL)
			}
			
			// 获取平台的 API Key
			apiKeys, err := s.modelManager.GetAvailableAPIKeys(modelCode)
			if err == nil && len(apiKeys) > 0 {
				apiKey = apiKeys[0].APIKey
				logger.Printf("✅ 从平台获取到 API Key (model_code: %s, KeyID: %d)", modelCode, apiKeys[0].ID)
			} else {
				logger.Printf("⚠️  无法从平台获取 API Key (model_code: %s): %v", modelCode, err)
			}
		} else {
			logger.Printf("⚠️  无法获取模型配置 (model_code: %s): %v", modelCode, err)
		}
	}
	
	// 如果仍然没有 API Key，记录警告但继续尝试（某些 API 可能不需要 Key）
	if apiKey == "" {
		logger.Printf("⚠️  警告: API Key 为空，将尝试不使用 Key 调用 API")
	}
	
	// 调用相应的服务获取模型列表
	var rawResponse map[string]interface{}
	var err error
	// 从模型配置中获取模型列表端点（如果已配置）
	modelsEndpoint := ""
	if cfg, cfgErr := s.modelManager.GetModelConfig(provider); cfgErr == nil && cfg != nil {
		modelsEndpoint = cfg.ModelsEndpoint
	}
	
	switch provider {
	case "Anthropic":
		// 使用 Claude 服务
		rawResponse, err = s.claudeService.GetModels(ctx, baseURL, apiKey, modelsEndpoint)
	case "Gemini":
		// Gemini 使用特殊的认证方式，暂时使用 OpenAI 兼容方式
		// TODO: 如果需要，可以为 Gemini 创建专门的服务
		rawResponse, err = s.openaiService.GetModels(ctx, baseURL, apiKey, modelsEndpoint)
	default:
		// OpenAI 兼容 API（OpenAI, DeepSeek, MoonShot, xAI, Meta, Qwen, Doubao 等）
		rawResponse, err = s.openaiService.GetModels(ctx, baseURL, apiKey, modelsEndpoint)
	}
	
	// 如果获取失败，直接返回错误
	if err != nil {
		return nil, err
	}
	
	// 如果响应为空，返回空列表
	if rawResponse == nil {
		return map[string]interface{}{
			"object": "list",
			"data":   []interface{}{},
		}, nil
	}
	
	// 使用 formatConverter 规范化响应格式
	normalizedResponse, err := s.formatConverter.NormalizeModelsResponse(provider, rawResponse)
	if err != nil {
		logger.Printf("⚠️  规范化模型列表响应失败: %v，返回原始响应", err)
		// 如果规范化失败，返回原始响应
		return rawResponse, nil
	}
	
	logger.Printf("✅ 成功规范化 %s 模型列表响应", provider)
	return normalizedResponse, nil
}