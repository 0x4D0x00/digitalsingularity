// 模型维护服务：从 API 获取模型列表并同步到数据库

package maintenance

import (
	"context"
	"fmt"
	"log"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/silicoid/database"
	"digitalsingularity/backend/silicoid/interceptor"
)

// 获取logger
var logger = log.New(log.Writer(), "maintenance_service: ", log.LstdFlags)

// MaintenanceService 模型维护服务
type MaintenanceService struct {
	readWrite   *datahandle.CommonReadWriteService
	dbService   *database.SilicoidDataService
	interceptor *interceptor.SilicoIDInterceptor
}

// ModelMaintenanceService 模型维护服务（别名，用于向后兼容）
type ModelMaintenanceService = MaintenanceService

// NewMaintenanceService 创建模型维护服务实例
func NewMaintenanceService() *MaintenanceService {
	readWrite, _ := datahandle.NewCommonReadWriteService("database")
	dbService := database.NewSilicoidDataService(readWrite)
	interceptorService := interceptor.CreateInterceptor()

	service := &MaintenanceService{
		readWrite:   readWrite,
		dbService:   dbService,
		interceptor: interceptorService,
	}

	logger.Printf("初始化模型维护服务完成")
	return service
}

// NewModelMaintenanceService 创建模型维护服务实例（别名，用于向后兼容）
func NewModelMaintenanceService() *ModelMaintenanceService {
	return NewMaintenanceService()
}

// SyncProviderModels 同步指定提供商的模型列表
// provider: 提供商名称（如 "DeepSeek", "OpenAI", "Anthropic" 等）
// 如果 baseURL 和 apiKey 为空，将从数据库中的 ai_models 表获取配置
func (s *MaintenanceService) SyncProviderModels(provider string, baseURL ...string) error {
	ctx := context.Background()
	
	// 如果未提供 baseURL，从数据库获取配置
	var actualBaseURL, apiKey string
	if len(baseURL) == 0 || baseURL[0] == "" {
		// 从数据库获取该提供商的配置
		models, err := s.dbService.GetAllAIModelsWithFilters(false, "", provider, "")
		if err != nil || len(models) == 0 {
			return fmt.Errorf("无法从数据库获取 %s 的配置: %v", provider, err)
		}
		
		// 使用第一个模型的配置
		model := models[0]
		actualBaseURL = getStringValue(model["base_url"])
		modelCode := getStringValue(model["model_code"])
		
		// 获取 API Key
		if modelCode != "" {
			apiKeys, err := s.dbService.GetModelAPIKeys(modelCode)
			if err == nil && len(apiKeys) > 0 {
				apiKey = getStringValue(apiKeys[0]["api_key"])
			}
		}
		
		if actualBaseURL == "" || apiKey == "" {
			return fmt.Errorf("无法从数据库获取 %s 的完整配置 (baseURL: %s, apiKey: %s)", provider, actualBaseURL, func() string {
				if apiKey != "" {
					return "已设置"
				}
				return "未设置"
			}())
		}
	} else {
		actualBaseURL = baseURL[0]
		// 如果提供了 baseURL，尝试从请求中获取 apiKey，或者从数据库获取
		if len(baseURL) > 1 && baseURL[1] != "" {
			apiKey = baseURL[1]
		} else {
			// 从数据库获取
			models, err := s.dbService.GetAllAIModelsWithFilters(false, "", provider, "")
			if err == nil && len(models) > 0 {
				modelCode := getStringValue(models[0]["model_code"])
				if modelCode != "" {
					apiKeys, err := s.dbService.GetModelAPIKeys(modelCode)
					if err == nil && len(apiKeys) > 0 {
						apiKey = getStringValue(apiKeys[0]["api_key"])
					}
				}
			}
		}
	}
	
	return s.syncProviderModelsWithConfig(ctx, provider, actualBaseURL, apiKey)
}

// syncProviderModelsWithConfig 同步指定提供商的模型列表（内部方法）
func (s *MaintenanceService) syncProviderModelsWithConfig(ctx context.Context, provider string, baseURL string, apiKey string) error {
	logger.Printf("开始同步 %s 的模型列表 (baseURL: %s)", provider, baseURL)

	// 获取该提供商的 model_code（从 silicoid_models 表）
	providerModelCode := ""
	models, err := s.dbService.GetAllAIModelsWithFilters(false, "", provider, "")
	if err == nil && len(models) > 0 {
		providerModelCode = getStringValue(models[0]["model_code"])
	}
	if providerModelCode == "" {
		// 如果无法从数据库获取，尝试从已有的提供商模型中获取
		dbModels, _ := s.dbService.GetAllProviderModels(provider, map[string]interface{}{}, "id ASC")
		for _, dbModel := range dbModels {
			modelCode := getStringValue(dbModel["model_code"])
			if modelCode != "" {
				providerModelCode = modelCode
				break
			}
		}
	}
	if providerModelCode == "" {
		return fmt.Errorf("无法获取 %s 的 model_code，请先配置该提供商的模型", provider)
	}

	// 先获取数据库中该提供商的所有模型ID，以便在同步失败时进行软删除
	dbModels, err := s.dbService.GetAllProviderModels(provider, map[string]interface{}{}, "id ASC")
	if err != nil {
		logger.Printf("⚠️ 获取数据库模型列表失败，无法进行软删除: %v", err)
		return fmt.Errorf("获取数据库模型列表失败: %v", err)
	}

	// 提取所有模型代码，用于同步失败时的软删除
	allModelCodes := []string{}
	for _, dbModel := range dbModels {
		modelCode := getStringValue(dbModel["model_code"])
		if modelCode != "" {
			allModelCodes = append(allModelCodes, modelCode)
		}
	}

	// 1. 从 API 获取模型列表
	modelsResponse, err := s.interceptor.GetModelsFromAPI(ctx, provider, baseURL, apiKey)
	if err != nil {
		// 同步失败：软删除所有模型
		if len(allModelCodes) > 0 {
			logger.Printf("⚠️ 同步失败，软删除 %s 的所有 %d 个模型", provider, len(allModelCodes))
			softDeleteErr := s.dbService.DeleteProviderModelsBatch(provider, allModelCodes)
			if softDeleteErr != nil {
				logger.Printf("❌ 软删除模型失败: %v", softDeleteErr)
			} else {
				logger.Printf("✅ 已软删除 %s 的所有模型", provider)
			}
		}
		return fmt.Errorf("获取模型列表失败: %v", err)
	}

	// 2. 解析响应
	modelsData, ok := modelsResponse["data"].([]interface{})
	if !ok {
		// 同步失败：软删除所有模型
		if len(allModelCodes) > 0 {
			logger.Printf("⚠️ 同步失败（响应格式错误），软删除 %s 的所有 %d 个模型", provider, len(allModelCodes))
			softDeleteErr := s.dbService.DeleteProviderModelsBatch(provider, allModelCodes)
			if softDeleteErr != nil {
				logger.Printf("❌ 软删除模型失败: %v", softDeleteErr)
			} else {
				logger.Printf("✅ 已软删除 %s 的所有模型", provider)
			}
		}
		return fmt.Errorf("模型列表格式错误: data 字段不是数组")
	}

	logger.Printf("从 API 获取到 %d 个模型", len(modelsData))

	// 3. 提取模型名称列表（只包含支持工具调用的模型）
	apiModelNames := make(map[string]bool)
	for _, modelItem := range modelsData {
		modelMap, ok := modelItem.(map[string]interface{})
		if !ok {
			continue
		}

		// 获取模型 ID（通常是 model_name）
		modelID, ok := modelMap["id"].(string)
		if !ok {
			continue
		}

		// 检查模型是否支持工具调用
		supportsTools := false
		if supportedParams, ok := modelMap["supported_parameters"].([]interface{}); ok {
			for _, param := range supportedParams {
				if paramStr, ok := param.(string); ok && paramStr == "tools" {
					supportsTools = true
					break
				}
			}
		}

		// 只同步支持工具调用的模型
		if supportsTools {
			apiModelNames[modelID] = true
			logger.Printf("  - 发现支持工具调用的模型: %s", modelID)
		} else {
			logger.Printf("  - 跳过不支持工具调用的模型: %s", modelID)
		}
	}

	logger.Printf("数据库中 %s 有 %d 个模型（包括不可用的）", provider, len(dbModels))

	// 5. 构建数据库模型映射（model_name -> dbModel）
	dbModelMap := make(map[string]map[string]interface{})
	for _, dbModel := range dbModels {
		modelName := getStringValue(dbModel["model_name"])
		if modelName == "" {
			logger.Printf("  - 警告: 发现模型名称为空的记录，跳过: %+v", dbModel)
			continue
		}
		modelID := getIntValue(dbModel["id"])
		if modelID <= 0 {
			logger.Printf("  - 警告: 模型 %s 的 ID 无效 (%v, 类型: %T)，跳过", modelName, dbModel["id"], dbModel["id"])
			continue
		}
		dbModelMap[modelName] = dbModel
	}

	// 6. 处理 API 返回的模型：更新或创建
	modelsToUpdate := []database.ProviderModelUpdate{}
	modelsToCreate := []database.ProviderModel{}
	
	for modelName := range apiModelNames {
		if dbModel, exists := dbModelMap[modelName]; exists {
			// 模型已存在，检查是否需要更新状态
			modelCode := getStringValue(dbModel["model_code"])
			if modelCode == "" {
				logger.Printf("  - 警告: 模型 %s 的 model_code 为空，先更新 model_code 为: %s", modelName, providerModelCode)
				// model_code 为空，先通过 model_name 更新 model_code
				tableName := getProviderModelTableName(provider)
				if tableName != "" {
					dbName := s.dbService.GetDatabaseName()
					updateCodeQuery := fmt.Sprintf(`UPDATE %s.%s SET model_code = ? WHERE model_name = ? AND (model_code IS NULL OR model_code = '')`, 
						dbName, tableName)
					// 这里需要直接执行 SQL，因为 UpdateProviderModel 需要 model_code
					// 我们使用 readWrite 服务来执行
					updateResult := s.readWrite.ExecuteDb(updateCodeQuery, providerModelCode, modelName)
					if updateResult.IsSuccess() {
						logger.Printf("  - ✅ 成功更新模型 %s 的 model_code 为: %s", modelName, providerModelCode)
						modelCode = providerModelCode
					} else {
						logger.Printf("  - ⚠️ 更新模型 %s 的 model_code 失败: %v", modelName, updateResult.Error)
						// 如果更新失败，使用提供商的默认 model_code 继续
						modelCode = providerModelCode
					}
				} else {
					modelCode = providerModelCode
				}
			}

			isAvailable := getIntValue(dbModel["is_available"])
			deprecated := getIntValue(dbModel["deprecated"])

			if isAvailable != 1 || deprecated != 0 {
				logger.Printf("  - 模型 %s (model_code: %s) 已存在但状态不正确，更新为可用", modelName, modelCode)
				modelsToUpdate = append(modelsToUpdate, database.ProviderModelUpdate{
					Code: modelCode,
					Model: database.ProviderModel{
						IsAvailable: intPtr(1),
						Deprecated:  intPtr(0),
					},
				})
			} else {
				logger.Printf("  - 模型 %s (model_code: %s) 已存在且状态正确，无需更新", modelName, modelCode)
			}
		} else {
			// 模型不存在，需要创建
			logger.Printf("  - 发现新模型: %s，准备创建 (model_code: %s)", modelName, providerModelCode)
			modelsToCreate = append(modelsToCreate, database.ProviderModel{
				ModelCode:   providerModelCode,
				ModelName:   modelName,
				IsAvailable: intPtr(1),
				Deprecated:  intPtr(0),
			})
		}
	}

	// 7. 批量更新已存在但状态不正确的模型
	if len(modelsToUpdate) > 0 {
		logger.Printf("开始更新 %d 个模型的状态", len(modelsToUpdate))
		err := s.dbService.UpdateProviderModelsBatch(provider, modelsToUpdate)
		if err != nil {
			// 同步失败：软删除所有模型
			if len(allModelCodes) > 0 {
				logger.Printf("⚠️ 同步失败（更新模型状态失败），软删除 %s 的所有 %d 个模型", provider, len(allModelCodes))
				softDeleteErr := s.dbService.DeleteProviderModelsBatch(provider, allModelCodes)
				if softDeleteErr != nil {
					logger.Printf("❌ 软删除模型失败: %v", softDeleteErr)
				} else {
					logger.Printf("✅ 已软删除 %s 的所有模型", provider)
				}
			}
			return fmt.Errorf("更新模型状态失败: %v", err)
		}
		logger.Printf("✅ 成功更新 %d 个模型的状态", len(modelsToUpdate))
	}

	// 8. 批量创建新模型
	if len(modelsToCreate) > 0 {
		logger.Printf("开始创建 %d 个新模型", len(modelsToCreate))
		_, err := s.dbService.CreateProviderModelsBatch(provider, modelsToCreate)
		if err != nil {
			// 同步失败：软删除所有模型
			if len(allModelCodes) > 0 {
				logger.Printf("⚠️ 同步失败（创建模型失败），软删除 %s 的所有 %d 个模型", provider, len(allModelCodes))
				softDeleteErr := s.dbService.DeleteProviderModelsBatch(provider, allModelCodes)
				if softDeleteErr != nil {
					logger.Printf("❌ 软删除模型失败: %v", softDeleteErr)
				} else {
					logger.Printf("✅ 已软删除 %s 的所有模型", provider)
				}
			}
			return fmt.Errorf("创建模型失败: %v", err)
		}
		logger.Printf("✅ 成功创建 %d 个新模型", len(modelsToCreate))
	}

	// 9. 处理数据库中但不在 API 返回列表中的模型（同步成功时硬删除）
	modelsToDelete := []string{}
	for modelName, dbModel := range dbModelMap {
		if !apiModelNames[modelName] {
			modelCode := getStringValue(dbModel["model_code"])
			logger.Printf("  - 模型 %s (model_code: %s) 在 API 中不存在，标记为硬删除", modelName, modelCode)
			// 使用 model_name 而不是 model_code，因为同一个 model_code 可能对应多个模型
			modelsToDelete = append(modelsToDelete, modelName)
		}
	}

	// 10. 同步成功：硬删除不存在的模型（物理删除）
	if len(modelsToDelete) > 0 {
		logger.Printf("开始硬删除 %d 个不存在的模型", len(modelsToDelete))
		// 使用 model_name 来删除，确保精确删除指定的模型
		err := s.dbService.HardDeleteProviderModelsByNames(provider, modelsToDelete)
		if err != nil {
			// 硬删除失败，但同步已基本成功，只记录错误
			logger.Printf("⚠️ 硬删除模型失败: %v", err)
			return fmt.Errorf("硬删除模型失败: %v", err)
		}
		logger.Printf("✅ 成功硬删除 %d 个不存在的模型", len(modelsToDelete))
	} else {
		logger.Printf("✅ 所有模型都存在于 API 中，无需删除")
	}

	logger.Printf("✅ %s 模型同步完成", provider)
	return nil
}

// SyncAllProviderModels 同步所有提供商的模型列表
func (s *MaintenanceService) SyncAllProviderModels() error {
	ctx := context.Background()
	logger.Printf("开始同步所有提供商的模型列表")

	// 获取所有已配置的模型（从 ai_models 表）
	allModels, err := s.dbService.GetAllAIModelsWithFilters(false, "", "", "")
	if err != nil {
		return fmt.Errorf("获取已配置模型列表失败: %v", err)
	}

	// 按提供商分组
	providerConfigs := make(map[string]map[string]string) // provider -> {baseURL, apiKey, modelCode}
	for _, model := range allModels {
		provider := getStringValue(model["provider"])
		if provider == "" {
			continue
		}

		baseURL := getStringValue(model["base_url"])
		modelCode := getStringValue(model["model_code"])

		// 获取该模型的 API Key
		apiKey := ""
		if modelCode != "" {
			apiKeys, err := s.dbService.GetModelAPIKeys(modelCode)
			if err == nil && len(apiKeys) > 0 {
				apiKey = getStringValue(apiKeys[0]["api_key"])
			}
		}

		if baseURL != "" && apiKey != "" {
			if providerConfigs[provider] == nil {
				providerConfigs[provider] = make(map[string]string)
			}
			providerConfigs[provider]["baseURL"] = baseURL
			providerConfigs[provider]["apiKey"] = apiKey
			providerConfigs[provider]["modelCode"] = modelCode
		}
	}

	// 同步每个提供商
	successCount := 0
	failCount := 0
	for provider, config := range providerConfigs {
		logger.Printf("同步提供商: %s", provider)
		err := s.syncProviderModelsWithConfig(ctx, provider, config["baseURL"], config["apiKey"])
		if err != nil {
			logger.Printf("❌ 同步 %s 失败: %v", provider, err)
			failCount++
		} else {
			logger.Printf("✅ 同步 %s 成功", provider)
			successCount++
		}
	}

	logger.Printf("✅ 所有提供商同步完成: 成功 %d, 失败 %d", successCount, failCount)
	return nil
}

// 辅助函数：获取提供商模型表名
func getProviderModelTableName(provider string) string {
	tableMap := map[string]string{
		"Anthropic":          "models_anthropic",
		"OpenAI":             "models_openai",
		"DeepSeek":           "models_deepseek",
		"MoonShot":           "models_moonshot",
		"xAI":                "models_xai",
		"Meta":               "models_meta",
		"DigitalSingularity": "models_digitalsingularity",
		"Bytedance":          "models_bytedance",
		"Google":             "models_google",
		"Alibaba":            "models_alibaba",
		"OpenRouter":         "models_openrouter",
	}
	return tableMap[provider]
}

// 辅助函数：安全获取字符串值
func getStringValue(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

// 辅助函数：安全获取整数值
func getIntValue(value interface{}) int {
	if value == nil {
		return 0
	}
	
	// 处理各种整数类型
	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case string:
		// 处理字符串类型的数字
		var result int
		if _, err := fmt.Sscanf(v, "%d", &result); err == nil {
			return result
		}
		return 0
	case []byte:
		// 处理 MySQL 可能返回的 []byte 类型
		if len(v) == 0 {
			return 0
		}
		// 尝试解析为整数
		var result int
		if _, err := fmt.Sscanf(string(v), "%d", &result); err == nil {
			return result
		}
		return 0
	}
	
	return 0
}

// intPtr 返回 int 指针
func intPtr(i int) *int {
	return &i
}

