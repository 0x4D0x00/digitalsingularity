// AI模型配置管理服务
// 实现两层架构: Redis缓存 -> MySQL数据库

package manager

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/silicoid/database"
)

// ModelConfig 模型配置结构
type ModelConfig struct {
	ID              int     `json:"id"`
	ModelCode       string  `json:"model_code"`
	ModelName       string  `json:"model_name"`
	Endpoint        string  `json:"endpoint"`
	ModelsEndpoint  string  `json:"models_endpoint"`
	BaseURL         string  `json:"base_url"`
	UploadBaseURL   string  `json:"upload_base_url"`
	ModelType       string  `json:"model_type"`
	Provider        string  `json:"provider"`
	Status          int     `json:"status"`
	Priority        int     `json:"priority"`
	MaxTokens       int     `json:"max_tokens"`
	CostPer1kInput  float64 `json:"cost_per_1k_input"`
	CostPer1kOutput float64 `json:"cost_per_1k_output"`
}

// APIKeyConfig API密钥配置结构
type APIKeyConfig struct {
	ID              int       `json:"id"`
	ModelID         int       `json:"model_id"`
	ModelCode       string    `json:"model_code"`
	APIKey          string    `json:"api_key"`
	KeyName         string    `json:"key_name"`
	Status          int       `json:"status"`
	Priority        int       `json:"priority"`
	UsageCount      int       `json:"usage_count"`
	SuccessCount    int       `json:"success_count"`
	FailCount       int       `json:"fail_count"`
	LastUsedAt      time.Time `json:"last_used_at"`
	LastSuccessAt   time.Time `json:"last_success_at"`
	LastFailAt      time.Time `json:"last_fail_at"`
	FailReason      string    `json:"fail_reason"`
	RateLimitPerMin int       `json:"rate_limit_per_min"`
	RateLimitPerDay int       `json:"rate_limit_per_day"`
}

// ModelManager 模型管理器
type ModelManager struct {
	readWrite       *datahandle.CommonReadWriteService
	dbService       *database.SilicoidDataService
	cacheExpire     time.Duration // Redis缓存过期时间
	logger          *log.Logger
}

var (
	modelManagerInstance *ModelManager
	modelManagerOnce     sync.Once
)

// NewModelManager 创建模型管理器实例（已废弃，请使用 GetModelManager）
// 为了保持向后兼容，保留此函数，但内部调用 GetModelManager
func NewModelManager() *ModelManager {
	return GetModelManager()
}

// GetModelManager 获取模型管理器单例实例
// 使用单例模式确保整个应用只有一个 ModelManager 实例，避免重复加载配置
func GetModelManager() *ModelManager {
	modelManagerOnce.Do(func() {
		readWrite, err := datahandle.NewCommonReadWriteService("database")
		if err != nil {
			// 数据库连接失败，返回 nil
			return
		}
		
		dbService := database.NewSilicoidDataService(readWrite)
		
		modelManagerInstance = &ModelManager{
			readWrite:     readWrite,
			dbService:    dbService,
			cacheExpire:   1 * time.Hour, // 缓存1小时
			logger:        log.New(io.Discard, "", 0), // 禁用日志输出
		}
		
		// 启动时预加载所有模型配置（只执行一次）
		modelManagerInstance.preloadAllModelConfigs()
	})
	
	return modelManagerInstance
}


// preloadAllModelConfigs 启动时预加载所有模型配置到Redis
func (m *ModelManager) preloadAllModelConfigs() {
	m.logger.Printf("开始预加载所有模型配置...")
	
	// 从数据库获取所有启用的模型
	models, err := m.loadAllModelsFromDatabase()
	if err != nil {
		m.logger.Printf("预加载模型配置失败: %v", err)
		return
	}
	
	// 预加载每个模型的配置
	for _, model := range models {
		// 清理可能存在的错误缓存
		m.clearModelCache(model.ModelCode)
		
		// 从数据库重新加载并缓存
		modelConfig, err := m.loadModelFromDatabase(model.ModelCode)
		if err != nil {
			m.logger.Printf("预加载模型 %s 失败: %v", model.ModelCode, err)
			continue
		}
		
		// 缓存模型配置
		m.cacheModelConfig(model.ModelCode, modelConfig)
		
		// 预加载API密钥
		apiKeys, err := m.loadAPIKeysFromDatabase(model.ModelCode)
		if err != nil {
			m.logger.Printf("预加载模型 %s 的API密钥失败: %v", model.ModelCode, err)
		} else {
			m.cacheAPIKeys(model.ModelCode, apiKeys)
		}
		
		m.logger.Printf("✅ 预加载模型配置完成: %s -> %s", model.ModelCode, modelConfig.ModelName)
	}
	
	m.logger.Printf("🎉 所有模型配置预加载完成，共加载 %d 个模型", len(models))
}

// GetAllModels 获取所有启用的模型列表 (公开方法，供外部调用)
// 优先从Redis缓存获取，缓存未命中则从数据库加载
func (m *ModelManager) GetAllModels() ([]*ModelConfig, error) {
	return m.loadAllModelsFromDatabase()
}

// GetAllProviderModels 获取所有公司模型表中的所有可用模型
// 返回所有公司的所有可用模型，而不仅仅是已配置的模型
func (m *ModelManager) GetAllProviderModels() ([]map[string]interface{}, error) {
	if m.dbService == nil {
		return nil, fmt.Errorf("数据库服务未初始化")
	}
	
	return m.dbService.GetAllProviderModelsFromAllProviders()
}

// loadAllModelsFromDatabase 从数据库加载所有启用的模型列表
func (m *ModelManager) loadAllModelsFromDatabase() ([]*ModelConfig, error) {
	if m.dbService == nil {
		return nil, fmt.Errorf("数据库服务未初始化")
	}
	
	rows, err := m.dbService.GetAllModels()
	if err != nil {
		return nil, err
	}
	
	var models []*ModelConfig
	for _, row := range rows {
		model := &ModelConfig{
			ID:         getIntValue(row["id"]),
			ModelCode:  getStringValue(row["model_code"]),
			ModelName:  getStringValue(row["model_name"]),
			Endpoint:   getStringValue(row["endpoint"]),
			ModelsEndpoint: getStringValue(row["models_endpoint"]),
			BaseURL:    getStringValue(row["base_url"]),
			ModelType:  getStringValue(row["model_type"]),
			Provider:   getStringValue(row["provider"]),
			Status:     getIntValue(row["status"]),
			Priority:   getIntValue(row["priority"]),
		}
		
		if val, ok := row["max_tokens"].(int64); ok {
			model.MaxTokens = int(val)
		} else if val, ok := row["max_tokens"].(int); ok {
			model.MaxTokens = val
		}
		
		models = append(models, model)
	}
	
	return models, nil
}

// getModelIDByCode 根据模型代码获取模型ID
func (m *ModelManager) getModelIDByCode(modelCode string) (int, error) {
	if m.dbService == nil {
		return 0, fmt.Errorf("数据库服务未初始化")
	}
	
	return m.dbService.GetModelIDByCode(modelCode)
}

// 辅助函数：安全获取字符串值
func getStringValue(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

// 辅助函数：安全获取布尔值
func getBoolValue(value interface{}) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	if i, ok := value.(int64); ok {
		return i != 0
	}
	return false
}

// 辅助函数：安全获取整数值
func getIntValue(value interface{}) int {
	if i, ok := value.(int64); ok {
		return int(i)
	}
	if i, ok := value.(int); ok {
		return i
	}
	return 0
}

// findModelCodeByModelName 通过 model_name 查找对应的 model_code
func (m *ModelManager) findModelCodeByModelName(modelName string) (string, error) {
	if m.dbService == nil {
		return "", fmt.Errorf("数据库服务未初始化")
	}
	
	return m.dbService.FindModelCodeByModelName(modelName)
}

// GetModelConfig 获取模型配置 (两层架构)
// 支持通过 model_code 或 model_name 查询
// 逻辑：
//   1. 如果传入的是 model_name（如 deepseek-chat），先查找对应的 model_code
//   2. 如果传入的是 model_code（如 DeepSeek），直接使用
//   3. 使用 model_code 查询时，如果有多条记录，返回 id 最大的那条（最新记录）
// 缓存策略：
//   1. 先从Redis缓存获取
//   2. 缓存未命中则从数据库加载
func (m *ModelManager) GetModelConfig(modelCodeOrName string) (*ModelConfig, error) {
	// 首先尝试通过 model_name 查找对应的 model_code（前端通常传的是 model_name）
	// 如果查找失败，说明传入的可能就是 model_code，直接使用
	modelCode := modelCodeOrName
	actualModelCode, err := m.findModelCodeByModelName(modelCodeOrName)
	if err == nil && actualModelCode != "" {
		// 找到了对应的 model_code（用户传的是 model_name）
		modelCode = actualModelCode
		m.logger.Printf("✅ 通过 model_name 找到 model_code: %s -> %s", modelCodeOrName, modelCode)
	} else {
		// 没找到，说明传入的可能就是 model_code，直接使用（会返回数据库中 id 最大的记录）
		m.logger.Printf("ℹ️  未找到对应的 model_name，将 %s 作为 model_code 使用（将返回最新记录）", modelCodeOrName)
	}
	
	// 第一层: 尝试从Redis缓存获取
	cacheKey := fmt.Sprintf("model:config:%s", modelCode)
	result := m.readWrite.GetRedis(cacheKey)
	
	if result.IsSuccess() {
		var modelConfig ModelConfig
		jsonStr, _ := result.Data.(string)
		if err := json.Unmarshal([]byte(jsonStr), &modelConfig); err == nil {
			// 验证缓存数据的正确性
			if modelConfig.ModelCode == modelCode {
				m.logger.Printf("从Redis缓存获取模型配置: %s", modelCode)
				return &modelConfig, nil
			} else {
				// 缓存数据不匹配，清理并重新加载
				m.logger.Printf("检测到缓存数据不匹配: 期望 %s, 实际 %s，清理缓存", modelCode, modelConfig.ModelCode)
				m.clearModelCache(modelCode)
			}
		}
	}
	
	// 第二层: 从数据库加载
	modelConfig, err := m.loadModelFromDatabase(modelCode)
	if err != nil {
		return nil, fmt.Errorf("数据库加载失败: %v (model_code: %s)", err, modelCode)
	}
	
	// 缓存到Redis
	m.cacheModelConfig(modelCode, modelConfig)
	m.logger.Printf("从数据库加载模型配置: %s (原始输入: %s)", modelCode, modelCodeOrName)
	return modelConfig, nil
}

// GetAvailableAPIKeys 获取模型的可用API密钥列表 (按优先级排序)
func (m *ModelManager) GetAvailableAPIKeys(modelCode string) ([]*APIKeyConfig, error) {
	// 先获取模型配置（验证模型是否存在）
	_, err := m.GetModelConfig(modelCode)
	if err != nil {
		return nil, err
	}
	
	// 第一层: 尝试从Redis缓存获取
	cacheKey := fmt.Sprintf("model:apikeys:%s", modelCode)
	result := m.readWrite.GetRedis(cacheKey)
	
	if result.IsSuccess() {
		var apiKeys []*APIKeyConfig
		jsonStr, _ := result.Data.(string)
		m.logger.Printf("DEBUG: 从Redis缓存获取到API密钥数据: %s, 数据长度: %d", modelCode, len(jsonStr))
		if err := json.Unmarshal([]byte(jsonStr), &apiKeys); err == nil {
			// 对从 Redis 读取的 API Key 也进行 trim 处理，确保兼容性
			for _, key := range apiKeys {
				if key != nil {
					key.APIKey = strings.TrimSpace(key.APIKey)
				}
			}
			m.logger.Printf("从Redis缓存获取API密钥列表: %s, 数量: %d", modelCode, len(apiKeys))
			return apiKeys, nil
		} else {
			m.logger.Printf("DEBUG: API密钥JSON反序列化失败: %v, 数据: %s", err, jsonStr)
		}
	} else {
		m.logger.Printf("DEBUG: Redis缓存获取失败: %v", result.Error)
	}
	
	// 第二层: 从数据库加载
	m.logger.Printf("DEBUG: 尝试从数据库加载API密钥: modelCode=%s", modelCode)
	apiKeys, err := m.loadAPIKeysFromDatabase(modelCode)
	if err == nil && len(apiKeys) > 0 {
		// 缓存到Redis
		m.cacheAPIKeys(modelCode, apiKeys)
		m.logger.Printf("从数据库加载API密钥列表: %s, 数量: %d", modelCode, len(apiKeys))
		return apiKeys, nil
	} else {
		m.logger.Printf("DEBUG: 数据库加载API密钥失败: err=%v, 数量=%d", err, len(apiKeys))
	}
	
	// 只从Redis缓存和数据库获取，不允许降级
	m.logger.Printf("ERROR: 无可用API密钥 - Redis缓存和数据库都无数据: %s", modelCode)
	return nil, fmt.Errorf("无可用API密钥: Redis缓存和数据库都无数据")
}

// loadModelFromDatabase 从数据库加载模型配置
func (m *ModelManager) loadModelFromDatabase(modelCode string) (*ModelConfig, error) {
	if m.dbService == nil {
		return nil, fmt.Errorf("数据库服务未初始化")
	}
	
	m.logger.Printf("查询模型配置: %s", modelCode)
	row, err := m.dbService.GetModelConfigByCode(modelCode)
	if err != nil {
		m.logger.Printf("数据库查询失败: %v", err)
		return nil, err
	}
	
	modelConfig := &ModelConfig{
		ID:         getIntValue(row["id"]),
		ModelCode:  getStringValue(row["model_code"]),
		ModelName:  getStringValue(row["model_name"]),
		Endpoint:   getStringValue(row["endpoint"]),
		ModelsEndpoint: getStringValue(row["models_endpoint"]),
		BaseURL:    getStringValue(row["base_url"]),
		ModelType:  getStringValue(row["model_type"]),
		Provider:   getStringValue(row["provider"]),
		Status:     getIntValue(row["status"]),
		Priority:   getIntValue(row["priority"]),
	}
	
	// 处理可选字段
	if val, ok := row["upload_base_url"].(string); ok {
		modelConfig.UploadBaseURL = val
	}
	if val, ok := row["max_tokens"].(int64); ok {
		modelConfig.MaxTokens = int(val)
	} else if val, ok := row["max_tokens"].(int); ok {
		modelConfig.MaxTokens = val
	}
	if val, ok := row["cost_per_1k_input"].(float64); ok {
		modelConfig.CostPer1kInput = val
	}
	if val, ok := row["cost_per_1k_output"].(float64); ok {
		modelConfig.CostPer1kOutput = val
	}
	
	return modelConfig, nil
}

// loadAPIKeysFromDatabase 从数据库加载API密钥列表
func (m *ModelManager) loadAPIKeysFromDatabase(modelCode string) ([]*APIKeyConfig, error) {
	if m.dbService == nil {
		return nil, fmt.Errorf("数据库服务未初始化")
	}
	
	m.logger.Printf("查询API密钥: modelCode=%s", modelCode)
	rows, err := m.dbService.GetModelAPIKeys(modelCode)
	if err != nil {
		m.logger.Printf("API密钥查询失败: %v", err)
		return nil, err
	}
	
	apiKeys := make([]*APIKeyConfig, 0, len(rows))
	for _, row := range rows {
		// 对 API Key 进行 trim 处理，去除前后空格和换行符
		rawAPIKey := getStringValue(row["api_key"])
		trimmedAPIKey := strings.TrimSpace(rawAPIKey)
		
		apiKey := &APIKeyConfig{
			ID:           getIntValue(row["id"]),
			ModelID:      getIntValue(row["model_id"]),
			APIKey:       trimmedAPIKey,
			Status:       getIntValue(row["status"]),
			Priority:     getIntValue(row["priority"]),
			UsageCount:   getIntValue(row["usage_count"]),
			SuccessCount: getIntValue(row["success_count"]),
			FailCount:    getIntValue(row["fail_count"]),
		}
		
		// 处理可选字段
		if val, ok := row["model_code"].(string); ok {
			apiKey.ModelCode = val
		}
		if val, ok := row["key_name"].(string); ok {
			apiKey.KeyName = val
		}
		if val, ok := row["fail_reason"].(string); ok {
			apiKey.FailReason = val
		}
		if val, ok := row["rate_limit_per_min"].(int64); ok {
			apiKey.RateLimitPerMin = int(val)
		} else if val, ok := row["rate_limit_per_min"].(int); ok {
			apiKey.RateLimitPerMin = val
		}
		if val, ok := row["rate_limit_per_day"].(int64); ok {
			apiKey.RateLimitPerDay = int(val)
		} else if val, ok := row["rate_limit_per_day"].(int); ok {
			apiKey.RateLimitPerDay = val
		}
		
		apiKeys = append(apiKeys, apiKey)
	}
	
	return apiKeys, nil
}


// cacheModelConfig 缓存模型配置到Redis
func (m *ModelManager) cacheModelConfig(modelCode string, config *ModelConfig) {
	jsonData, err := json.Marshal(config)
	if err != nil {
		return // 静默失败，不输出日志
	}
	
	cacheKey := fmt.Sprintf("model:config:%s", modelCode)
	result := m.readWrite.SetRedis(cacheKey, string(jsonData), m.cacheExpire)
	if !result.IsSuccess() {
		// 检查是否为Redis只读错误，如果是则静默跳过
		if result.Error != nil && result.Error.Error() == "READONLY You can't write against a read only replica." {
			return // 静默跳过只读副本错误
		}
	}
}

// cacheAPIKeys 缓存API密钥列表到Redis
func (m *ModelManager) cacheAPIKeys(modelCode string, apiKeys []*APIKeyConfig) {
	jsonData, err := json.Marshal(apiKeys)
	if err != nil {
		return // 静默失败，不输出日志
	}
	
	cacheKey := fmt.Sprintf("model:apikeys:%s", modelCode)
	result := m.readWrite.SetRedis(cacheKey, string(jsonData), m.cacheExpire)
	if !result.IsSuccess() {
		// 检查是否为Redis只读错误，如果是则静默跳过
		if result.Error != nil && result.Error.Error() == "READONLY You can't write against a read only replica." {
			return // 静默跳过只读副本错误
		}
	}
}

// clearModelCache 清理指定模型的Redis缓存
func (m *ModelManager) clearModelCache(modelCode string) {
	configKey := fmt.Sprintf("model:config:%s", modelCode)
	apiKeysKey := fmt.Sprintf("model:apikeys:%s", modelCode)
	
	// 删除模型配置缓存
	result1 := m.readWrite.DeleteRedis(configKey)
	if !result1.IsSuccess() && result1.Error != nil && result1.Error.Error() == "READONLY You can't write against a read only replica." {
		return // 静默跳过只读副本错误
	}
	
	// 删除API密钥缓存
	result2 := m.readWrite.DeleteRedis(apiKeysKey)
	if !result2.IsSuccess() && result2.Error != nil && result2.Error.Error() == "READONLY You can't write against a read only replica." {
		return // 静默跳过只读副本错误
	}
}

// reloadModelConfig 重新加载模型配置（清理缓存后从数据库重新加载）
func (m *ModelManager) reloadModelConfig(modelCode string) error {
	m.logger.Printf("开始重新加载模型配置: %s", modelCode)
	
	// 清理现有缓存
	m.clearModelCache(modelCode)
	
	// 从数据库重新加载
	modelConfig, err := m.loadModelFromDatabase(modelCode)
	if err != nil {
		m.logger.Printf("重新加载模型配置失败: %v", err)
		return err
	}
	
	// 重新缓存
	m.cacheModelConfig(modelCode, modelConfig)
	
	// 获取模型ID并重新加载API密钥
	apiKeys, err := m.loadAPIKeysFromDatabase(modelCode)
	if err != nil {
		m.logger.Printf("重新加载API密钥失败: %v", err)
		return err
	}
	
	// 重新缓存API密钥
	m.cacheAPIKeys(modelCode, apiKeys)
	
	m.logger.Printf("成功重新加载模型配置: %s -> %s", modelCode, modelConfig.ModelName)
	return nil
}

// UpdateKeyStatus 更新API密钥状态 (成功/失败)
func (m *ModelManager) UpdateKeyStatus(keyID int, success bool, errMsg string) error {
	if m.dbService == nil {
		return fmt.Errorf("数据库服务未初始化")
	}
	
	// 使用 database 包的方法更新密钥使用统计
	var failReason *string
	if !success && errMsg != "" {
		failReason = &errMsg
	}
	
	err := m.dbService.UpdateModelApiKeyUsage(keyID, success, failReason)
	if err != nil {
		return err
	}
	
	m.logger.Printf("更新密钥状态: ID=%d, 成功=%v", keyID, success)
	
	// 如果失败次数过多，自动禁用密钥
	if !success {
		m.checkAndDisableKey(keyID)
	}
	
	return nil
}

// checkAndDisableKey 检查并禁用失败次数过多的密钥
func (m *ModelManager) checkAndDisableKey(keyID int) {
	if m.dbService == nil {
		return
	}
	
	// 使用 database 包的方法获取密钥信息
	keyInfo, err := m.dbService.GetModelApiKeyByID(keyID)
	if err != nil {
		return
	}
	
	failCount := getIntValue(keyInfo["fail_count"])
	successCount := getIntValue(keyInfo["success_count"])
	
	// 如果连续失败超过10次，或失败率超过50%，则禁用密钥
	if failCount >= 10 || (failCount > 5 && float64(failCount)/float64(failCount+successCount) > 0.5) {
		updates := map[string]interface{}{
			"status":     0,
			"fail_reason": "连续失败次数过多，已自动禁用",
		}
		err := m.dbService.UpdateModelApiKey(keyID, updates)
		if err != nil {
			m.logger.Printf("禁用密钥失败: ID=%d, 错误=%v", keyID, err)
		} else {
			m.logger.Printf("密钥已自动禁用: ID=%d, 失败次数=%d", keyID, failCount)
		}
	}
}

// InvalidateCache 使缓存失效 (当数据库更新时调用)
func (m *ModelManager) InvalidateCache(modelCode string) {
	// 删除模型配置缓存
	configKey := fmt.Sprintf("model:config:%s", modelCode)
	result1 := m.readWrite.DeleteRedis(configKey)
	if !result1.IsSuccess() && result1.Error != nil && result1.Error.Error() == "READONLY You can't write against a read only replica." {
		return // 静默跳过只读副本错误
	}
	
	// 删除API密钥缓存
	keysKey := fmt.Sprintf("model:apikeys:%s", modelCode)
	result2 := m.readWrite.DeleteRedis(keysKey)
	if !result2.IsSuccess() && result2.Error != nil && result2.Error.Error() == "READONLY You can't write against a read only replica." {
		return // 静默跳过只读副本错误
	}
}

// RefreshCache 主动刷新缓存
func (m *ModelManager) RefreshCache(modelCode string) error {
	// 重新加载模型配置
	modelConfig, err := m.loadModelFromDatabase(modelCode)
	if err != nil {
		return err
	}
	m.cacheModelConfig(modelCode, modelConfig)
	
	// 重新加载API密钥
	apiKeys, err := m.loadAPIKeysFromDatabase(modelCode)
	if err != nil {
		return err
	}
	m.cacheAPIKeys(modelCode, apiKeys)
	
	m.logger.Printf("已刷新模型缓存: %s", modelCode)
	return nil
}

