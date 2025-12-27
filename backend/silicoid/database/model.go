package database

import (
	"fmt"
	"log"
	"strings"

	"digitalsingularity/backend/common/utils/datahandle"
)

// GetModelConfigByProvider 根据提供商获取模型配置
func (s *SilicoidDataService) GetModelConfigByProvider(provider string) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取模型配置异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT id, model_code, endpoint, models_endpoint, base_url, upload_base_url,
		       model_type, provider, status, priority, max_tokens,
		       cost_per_1k_input, cost_per_1k_output
		FROM %s.silicoid_models
		WHERE provider = ? AND status = 1
		ORDER BY priority ASC, id ASC
		LIMIT 1
	`, s.dbName)

	opResult := s.readWrite.QueryDb(query, provider)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询模型配置失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 %s 模型配置", provider)
	}

	return rows[0], nil
}

// GetModelConfigByCode 根据模型代码获取模型配置
// 如果同一个 model_code 有多条记录，返回 id 最大的那条（最新的记录）
func (s *SilicoidDataService) GetModelConfigByCode(modelCode string) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取模型配置异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT id, model_code, endpoint, models_endpoint, base_url, upload_base_url,
		       model_type, provider, status, priority, max_tokens,
		       cost_per_1k_input, cost_per_1k_output
		FROM %s.silicoid_models
		WHERE model_code = ? AND status = 1
		ORDER BY id DESC
		LIMIT 1
	`, s.dbName)

	opResult := s.readWrite.QueryDb(query, modelCode)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询模型配置失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 model_code=%s 的模型配置", modelCode)
	}

	row := rows[0]
	provider := getStringValue(row["provider"])
	modelCode = getStringValue(row["model_code"])

	// 通过 JOIN 查询获取 model_name（获取最新的模型）
	// 优先使用 latest = 1 的模型，如果没有则使用 id 最大的那条作为降级方案
	if provider != "" && modelCode != "" {
		tableName := getProviderModelTableName(provider)
		if tableName != "" {
			// 优先查询 latest = 1 的模型
			nameQuery := fmt.Sprintf(`
				SELECT model_name 
				FROM %s.%s 
				WHERE model_code = ? AND is_available = 1 AND deprecated = 0 AND latest = 1
				LIMIT 1
			`, s.dbName, tableName)
			nameResult := s.readWrite.QueryDb(nameQuery, modelCode)
			if nameResult.IsSuccess() {
				if nameRows, ok := nameResult.Data.([]map[string]interface{}); ok && len(nameRows) > 0 {
					modelName := getStringValue(nameRows[0]["model_name"])
					row["model_name"] = modelName
				} else {
					// 如果没有 latest = 1 的模型，降级使用 id 最大的那条
					fallbackQuery := fmt.Sprintf(`
						SELECT model_name 
						FROM %s.%s 
						WHERE model_code = ? AND is_available = 1 AND deprecated = 0
						ORDER BY id DESC
						LIMIT 1
					`, s.dbName, tableName)
					fallbackResult := s.readWrite.QueryDb(fallbackQuery, modelCode)
					if fallbackResult.IsSuccess() {
						if fallbackRows, ok := fallbackResult.Data.([]map[string]interface{}); ok && len(fallbackRows) > 0 {
							modelName := getStringValue(fallbackRows[0]["model_name"])
							row["model_name"] = modelName
						}
					}
				}
			}
		}
	}

	return row, nil
}

// GetAllModels 获取所有启用的模型列表
func (s *SilicoidDataService) GetAllModels() ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取所有模型异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT id, model_code, endpoint, models_endpoint, base_url, model_type, provider, 
		       status, priority, max_tokens
		FROM %s.silicoid_models
		WHERE status = 1
		ORDER BY priority DESC, model_code ASC
	`, s.dbName)

	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询模型列表失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查询结果格式错误")
	}

	// 为每个模型获取 model_name（使用 JOIN 优化）
	// 按 provider 分组批量查询，减少数据库查询次数
	providerGroups := make(map[string][]int)
	for i, row := range rows {
		provider := getStringValue(row["provider"])
		modelCode := getStringValue(row["model_code"])
		if provider != "" && modelCode != "" {
			providerGroups[provider] = append(providerGroups[provider], i)
		}
	}

	// 批量查询每个 provider 的 model_name
	for provider, indices := range providerGroups {
		tableName := getProviderModelTableName(provider)
		if tableName == "" {
			continue
		}

		// 收集该 provider 的所有 model_code
		modelCodes := make([]string, 0, len(indices))
		for _, idx := range indices {
			modelCode := getStringValue(rows[idx]["model_code"])
			if modelCode != "" {
				modelCodes = append(modelCodes, modelCode)
			}
		}

		if len(modelCodes) == 0 {
			continue
		}

		// 构建 IN 查询
		placeholders := make([]string, len(modelCodes))
		args := make([]interface{}, len(modelCodes))
		for i, code := range modelCodes {
			placeholders[i] = "?"
			args[i] = code
		}

		nameQuery := fmt.Sprintf(`
			SELECT model_code, model_name 
			FROM %s.%s 
			WHERE model_code IN (%s) AND is_available = 1 AND deprecated = 0
		`, s.dbName, tableName, strings.Join(placeholders, ", "))

		nameResult := s.readWrite.QueryDb(nameQuery, args...)
		if nameResult.IsSuccess() {
			if nameRows, ok := nameResult.Data.([]map[string]interface{}); ok {
				// 创建 model_code -> model_name 的映射
				nameMap := make(map[string]string)
				for _, nameRow := range nameRows {
					code := getStringValue(nameRow["model_code"])
					name := getStringValue(nameRow["model_name"])
					if code != "" && name != "" {
						nameMap[code] = name
					}
				}

				// 更新 rows
				for _, idx := range indices {
					modelCode := getStringValue(rows[idx]["model_code"])
					if name, exists := nameMap[modelCode]; exists {
						rows[idx]["model_name"] = name
					}
				}
			}
		}
	}

	return rows, nil
}

// GetModelIDByCode 根据模型代码获取模型ID
func (s *SilicoidDataService) GetModelIDByCode(modelCode string) (int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取模型ID异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT id
		FROM %s.silicoid_models
		WHERE model_code = ?
	`, s.dbName)

	opResult := s.readWrite.QueryDb(query, modelCode)
	if !opResult.IsSuccess() {
		return 0, fmt.Errorf("查询模型ID失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return 0, fmt.Errorf("模型不存在: %s", modelCode)
	}

	modelID := getIntValue(rows[0]["id"])
	if modelID == 0 {
		return 0, fmt.Errorf("无效的模型ID: %s", modelCode)
	}

	return modelID, nil
}

// FindModelCodeByModelName 通过 model_name 查找对应的 model_code
func (s *SilicoidDataService) FindModelCodeByModelName(modelName string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("查找模型代码异常: %v", r)
		}
	}()

	// 在所有公司模型表中查询，直接通过 model_code JOIN
	providerTables := []struct {
		tableName string
		provider  string
	}{
		{"models_anthropic", "Anthropic"},
		{"models_openai", "OpenAI"},
		{"models_deepseek", "DeepSeek"},
		{"models_moonshot", "MoonShot"},
		{"models_xai", "xAI"},
		{"models_meta", "Meta"},
		{"models_digitalsingularity", "DigitalSingularity"},
		{"models_bytedance", "Bytedance"},
		{"models_google", "Google"},
		{"models_alibaba", "Alibaba"},
		{"models_openrouter", "OpenRouter"},
	}

	for _, pt := range providerTables {
		// 直接通过 JOIN 查询，一次性获取 model_code
		query := fmt.Sprintf(`
			SELECT pm.model_code
			FROM %s.%s pm
			INNER JOIN %s.silicoid_models am ON pm.model_code = am.model_code
			WHERE pm.model_name = ? AND pm.is_available = 1 AND pm.deprecated = 0 AND am.status = 1
			LIMIT 1
		`, s.dbName, pt.tableName, s.dbName)

		opResult := s.readWrite.QueryDb(query, modelName)
		if opResult.IsSuccess() {
			if rows, ok := opResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
				modelCode := getStringValue(rows[0]["model_code"])
				if modelCode != "" {
					log.Printf("✅ 通过 model_name 找到 model_code: %s (provider: %s) -> %s", modelName, pt.provider, modelCode)
					return modelCode, nil
				}
			}
		}
	}

	return "", fmt.Errorf("未找到对应的 model_code: %s", modelName)
}

// GetAllProviderModelsFromAllProviders 获取所有公司模型表中的所有可用模型
func (s *SilicoidDataService) GetAllProviderModelsFromAllProviders() ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取所有提供商模型异常: %v", r)
		}
	}()

	allModels := make([]map[string]interface{}, 0)

	// 定义所有公司的模型表及其对应的 provider 名称
	providerTables := []struct {
		tableName string
		provider  string
	}{
		{"models_anthropic", "Anthropic"},
		{"models_openai", "OpenAI"},
		{"models_deepseek", "DeepSeek"},
		{"models_moonshot", "MoonShot"},
		{"models_xai", "xAI"},
		{"models_meta", "Meta"},
		{"models_digitalsingularity", "DigitalSingularity"},
		{"models_bytedance", "Bytedance"},
		{"models_google", "Google"},
		{"models_alibaba", "Alibaba"},
		{"models_openrouter", "OpenRouter"},
	}

	// 从每个公司模型表中读取所有可用的模型（使用 JOIN 确保只返回已启用的模型）
	for _, pt := range providerTables {
		query := fmt.Sprintf(`
			SELECT pm.model_code, pm.model_name, pm.model_display_name, pm.description, 
			       pm.is_available, pm.deprecated
			FROM %s.%s pm
			INNER JOIN %s.silicoid_models am ON pm.model_code = am.model_code
			WHERE pm.is_available = 1 AND pm.deprecated = 0 AND am.status = 1
		`, s.dbName, pt.tableName, s.dbName)

		opResult := s.readWrite.QueryDb(query)
		if !opResult.IsSuccess() {
			log.Printf("⚠️ 查询 %s 模型表失败: %v", pt.tableName, opResult.Error)
			continue // 继续查询其他表，不中断
		}

		rows, ok := opResult.Data.([]map[string]interface{})
		if !ok {
			log.Printf("⚠️ %s 模型表查询结果格式错误", pt.tableName)
			continue
		}

		for _, row := range rows {
			modelName := getStringValue(row["model_name"])
			modelCode := getStringValue(row["model_code"])
			if modelName == "" || modelCode == "" {
				continue
			}

			// 为 DigitalSingularity 提供商的模型提供固定的对外展示 ID
			displayModelName := modelName
			if pt.provider == "DigitalSingularity" {
				displayModelName = "PotAGI"
			}

			allModels = append(allModels, map[string]interface{}{
				"id":           displayModelName, // 使用 model_name 作为 id（符合 OpenAI API 标准）
				"model_code":   modelCode,
				"model_name":   displayModelName,
				"display_name": getStringValue(row["model_display_name"]),
				"provider":     pt.provider,
				"description":  getStringValue(row["description"]),
				"available":    getIntValue(row["is_available"]) == 1,
			})
		}

		log.Printf("✅ 从 %s 加载了 %d 个模型", pt.tableName, len(rows))
	}

	return allModels, nil
}

// clearAIModelCache 清除AI模型的Redis缓存
// 参数:
//   modelCode: 模型代码（必填）
func (s *SilicoidDataService) clearAIModelCache(modelCode string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("清除AI模型缓存异常: %v", r)
		}
	}()

	if modelCode == "" {
		return
	}

	// 清除 ModelManager 使用的缓存
	// model:config:{modelCode} - 模型配置缓存
	configKey := fmt.Sprintf("model:config:%s", modelCode)
	result1 := s.readWrite.DeleteRedis(configKey)
	if !result1.IsSuccess() {
		if result1.Error != nil && result1.Error.Error() != "READONLY You can't write against a read only replica." {
			log.Printf("清除模型配置缓存失败 (modelCode=%s): %v", modelCode, result1.Error)
		}
	} else {
		log.Printf("✅ 成功清除模型配置缓存: modelCode=%s", modelCode)
	}

	// model:apikeys:{modelCode} - API密钥缓存
	apiKeysKey := fmt.Sprintf("model:apikeys:%s", modelCode)
	result2 := s.readWrite.DeleteRedis(apiKeysKey)
	if !result2.IsSuccess() {
		if result2.Error != nil && result2.Error.Error() != "READONLY You can't write against a read only replica." {
			log.Printf("清除模型API密钥缓存失败 (modelCode=%s): %v", modelCode, result2.Error)
		}
	} else {
		log.Printf("✅ 成功清除模型API密钥缓存: modelCode=%s", modelCode)
	}
}

// ==================== AI Models CRUD 操作 ====================

// CreateAIModel 创建新的AI模型
// 参数:
//   modelCode: 模型代码（必填，唯一）
//   endpoint: API端点（必填）
//   baseURL: 基础URL（必填）
//   uploadBaseURL: 上传文件的基础URL（可选）
//   modelType: 模型类型，'external' 或 'internal'（默认 'internal'）
//   provider: 提供商（可选）
//   status: 状态，1-启用，0-禁用（默认 1）
//   priority: 优先级，数值越大优先级越高（默认 0）
//   maxTokens: 最大token数（可选）
//   costPer1KInput: 每1K输入token成本（可选）
//   costPer1KOutput: 每1K输出token成本（可选）
//   description: 模型描述（可选）
// 返回: 新创建的模型ID和错误信息
func (s *SilicoidDataService) CreateAIModel(
	modelCode, endpoint, baseURL string,
	uploadBaseURL *string,
	modelType string,
	provider *string,
	status *int,
	priority *int,
	maxTokens *int,
	costPer1KInput *float64,
	costPer1KOutput *float64,
	description *string,
) (int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("创建AI模型异常: %v", r)
		}
	}()

	// 验证必填字段
	if modelCode == "" {
		return 0, fmt.Errorf("model_code 不能为空")
	}
	if endpoint == "" {
		return 0, fmt.Errorf("endpoint 不能为空")
	}
	if baseURL == "" {
		return 0, fmt.Errorf("base_url 不能为空")
	}

	// 设置默认值
	if modelType == "" {
		modelType = "internal"
	}
	if status == nil {
		defaultStatus := 1
		status = &defaultStatus
	}
	if priority == nil {
		defaultPriority := 0
		priority = &defaultPriority
	}

	// 检查 model_code 是否已存在
	checkQuery := fmt.Sprintf(`SELECT id FROM %s.silicoid_models WHERE model_code = ?`, s.dbName)
	checkResult := s.readWrite.QueryDb(checkQuery, modelCode)
	if checkResult.IsSuccess() {
		if rows, ok := checkResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			return 0, fmt.Errorf("model_code %s 已存在", modelCode)
		}
	}

	// 构建插入SQL
	query := fmt.Sprintf(`
		INSERT INTO %s.silicoid_models 
		(model_code, endpoint, base_url, upload_base_url, model_type, provider, 
		 status, priority, max_tokens, cost_per_1k_input, 
		 cost_per_1k_output, description)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.dbName)

	// 执行插入
	opResult := s.readWrite.ExecuteDb(query,
		modelCode, endpoint, baseURL, uploadBaseURL, modelType, provider,
		*status, *priority, maxTokens, costPer1KInput,
		costPer1KOutput, description,
	)

	if !opResult.IsSuccess() {
		return 0, fmt.Errorf("创建AI模型失败: %v", opResult.Error)
	}

	// 获取插入的ID
	idQuery := fmt.Sprintf(`SELECT id FROM %s.silicoid_models WHERE model_code = ?`, s.dbName)
	idResult := s.readWrite.QueryDb(idQuery, modelCode)
	if idResult.IsSuccess() {
		if rows, ok := idResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			modelID := getIntValue(rows[0]["id"])
			log.Printf("✅ 成功创建AI模型: model_code=%s, id=%d", modelCode, modelID)
			
			// 清除Redis缓存，让ModelManager重新加载
			s.clearAIModelCache(modelCode)
			
			return modelID, nil
		}
	}

	return 0, fmt.Errorf("创建成功但无法获取模型ID")
}

// GetAIModelByID 根据ID获取AI模型详细信息
// 参数:
//   id: 模型ID
// 返回: 模型信息映射和错误信息
func (s *SilicoidDataService) GetAIModelByID(id int) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取AI模型异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT id, model_code, endpoint, base_url, upload_base_url, model_type,
		       provider, status, priority, max_tokens,
		       cost_per_1k_input, cost_per_1k_output, description,
		       created_at, updated_at
		FROM %s.silicoid_models
		WHERE id = ?
	`, s.dbName)

	opResult := s.readWrite.QueryDb(query, id)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询AI模型失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 id=%d 的AI模型", id)
	}

	return rows[0], nil
}

// GetAIModelByCode 根据model_code获取AI模型详细信息（包含所有字段）
// 参数:
//   modelCode: 模型代码
//   includeDisabled: 是否包含已禁用的模型（默认false，只查询启用的）
// 返回: 模型信息映射和错误信息
func (s *SilicoidDataService) GetAIModelByCode(modelCode string, includeDisabled bool) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取AI模型异常: %v", r)
		}
	}()

	var query string
	if includeDisabled {
		query = fmt.Sprintf(`
			SELECT id, model_code, endpoint, base_url, upload_base_url, model_type,
			       provider, status, priority, max_tokens,
			       cost_per_1k_input, cost_per_1k_output, description,
			       created_at, updated_at
			FROM %s.silicoid_models
			WHERE model_code = ?
			ORDER BY priority DESC
			LIMIT 1
		`, s.dbName)
	} else {
		query = fmt.Sprintf(`
			SELECT id, model_code, endpoint, base_url, upload_base_url, model_type,
			       provider, status, priority, max_tokens,
			       cost_per_1k_input, cost_per_1k_output, description,
			       created_at, updated_at
			FROM %s.silicoid_models
			WHERE model_code = ? AND status = 1
			ORDER BY priority DESC
			LIMIT 1
		`, s.dbName)
	}

	opResult := s.readWrite.QueryDb(query, modelCode)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询AI模型失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 model_code=%s 的AI模型", modelCode)
	}

	return rows[0], nil
}

// GetAllAIModelsWithFilters 获取所有AI模型列表（支持过滤条件）
// 参数:
//   includeDisabled: 是否包含已禁用的模型（默认false）
//   modelType: 模型类型过滤，'external' 或 'internal'，空字符串表示不过滤
//   provider: 提供商过滤，空字符串表示不过滤
//   orderBy: 排序字段，默认 'priority DESC, id ASC'
// 返回: 模型信息列表和错误信息
func (s *SilicoidDataService) GetAllAIModelsWithFilters(
	includeDisabled bool,
	modelType string,
	provider string,
	orderBy string,
) ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取AI模型列表异常: %v", r)
		}
	}()

	// 构建WHERE条件
	whereConditions := []string{}
	args := []interface{}{}

	if !includeDisabled {
		whereConditions = append(whereConditions, "status = ?")
		args = append(args, 1)
	}

	if modelType != "" {
		whereConditions = append(whereConditions, "model_type = ?")
		args = append(args, modelType)
	}

	if provider != "" {
		whereConditions = append(whereConditions, "provider = ?")
		args = append(args, provider)
	}

	whereClause := ""
	if len(whereConditions) > 0 {
		whereClause = "WHERE " + whereConditions[0]
		for i := 1; i < len(whereConditions); i++ {
			whereClause += " AND " + whereConditions[i]
		}
	}

	// 设置默认排序
	if orderBy == "" {
		orderBy = "priority DESC, id ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, model_code, endpoint, base_url, upload_base_url, model_type,
		       provider, status, priority, max_tokens,
		       cost_per_1k_input, cost_per_1k_output, description,
		       created_at, updated_at
		FROM %s.silicoid_models
		%s
		ORDER BY %s
	`, s.dbName, whereClause, orderBy)

	var opResult *datahandle.OperationResult
	if len(args) > 0 {
		opResult = s.readWrite.QueryDb(query, args...)
	} else {
		opResult = s.readWrite.QueryDb(query)
	}

	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询AI模型列表失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查询结果格式错误")
	}

	return rows, nil
}

// UpdateAIModel 更新AI模型信息
// 参数:
//   id: 模型ID
//   updates: 要更新的字段映射，可以包含以下字段：
//     - endpoint, base_url, upload_base_url, model_type, provider,
//       status, priority, max_tokens,
//       cost_per_1k_input, cost_per_1k_output, description
// 返回: 错误信息
func (s *SilicoidDataService) UpdateAIModel(id int, updates map[string]interface{}) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("更新AI模型异常: %v", r)
		}
	}()

	if len(updates) == 0 {
		return fmt.Errorf("没有要更新的字段")
	}

	// 检查模型是否存在
	_, err := s.GetAIModelByID(id)
	if err != nil {
		return fmt.Errorf("模型不存在: %v", err)
	}

	// 允许更新的字段列表
	allowedFields := map[string]bool{
		"endpoint":           true,
		"base_url":           true,
		"upload_base_url":    true,
		"model_type":         true,
		"provider":           true,
		"status":             true,
		"priority":           true,
		"max_tokens":         true,
		"cost_per_1k_input":  true,
		"cost_per_1k_output": true,
		"description":        true,
	}

	// 构建SET子句
	setClause := []string{}
	args := []interface{}{}

	for field, value := range updates {
		if !allowedFields[field] {
			continue // 跳过不允许的字段
		}

		// 特殊处理：如果字段值为 nil，设置为 NULL
		if value == nil {
			setClause = append(setClause, fmt.Sprintf("%s = NULL", field))
		} else {
			setClause = append(setClause, fmt.Sprintf("%s = ?", field))
			args = append(args, value)
		}
	}

	if len(setClause) == 0 {
		return fmt.Errorf("没有有效的更新字段")
	}

	// 添加 id 参数
	args = append(args, id)

	// 构建完整的 SET 子句
	setClauseStr := setClause[0]
	for i := 1; i < len(setClause); i++ {
		setClauseStr += ", " + setClause[i]
	}

	query := fmt.Sprintf(`
		UPDATE %s.silicoid_models
		SET %s
		WHERE id = ?
	`, s.dbName, setClauseStr)

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("更新AI模型失败: %v", opResult.Error)
	}

	// 获取 model_code 以便清除缓存
	modelInfo, err := s.GetAIModelByID(id)
	if err == nil {
		modelCode := getStringValue(modelInfo["model_code"])
		// 清除Redis缓存，让ModelManager重新加载
		s.clearAIModelCache(modelCode)
	}

	log.Printf("✅ 成功更新AI模型: id=%d", id)
	return nil
}

// DeleteAIModel 软删除AI模型（设置status=0）
// 参数:
//   id: 模型ID
// 返回: 错误信息
func (s *SilicoidDataService) DeleteAIModel(id int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("删除AI模型异常: %v", r)
		}
	}()

	// 检查模型是否存在并获取 model_code 以便清除缓存
	modelInfo, err := s.GetAIModelByID(id)
	if err != nil {
		return fmt.Errorf("模型不存在: %v", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s.silicoid_models
		SET status = 0
		WHERE id = ?
	`, s.dbName)

	// 清除Redis缓存，让ModelManager重新加载
	modelCode := getStringValue(modelInfo["model_code"])
	s.clearAIModelCache(modelCode)

	opResult := s.readWrite.ExecuteDb(query, id)
	if !opResult.IsSuccess() {
		return fmt.Errorf("删除AI模型失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功软删除AI模型: id=%d", id)
	return nil
}

// HardDeleteAIModel 硬删除AI模型（从数据库中物理删除）
// 警告：此操作不可恢复，请谨慎使用
// 参数:
//   id: 模型ID
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteAIModel(id int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("硬删除AI模型异常: %v", r)
		}
	}()

	// 检查模型是否存在并获取 model_code 以便清除缓存
	modelInfo, err := s.GetAIModelByID(id)
	if err != nil {
		return fmt.Errorf("模型不存在: %v", err)
	}

	// 清除Redis缓存，让ModelManager重新加载
	modelCode := getStringValue(modelInfo["model_code"])
	s.clearAIModelCache(modelCode)

	query := fmt.Sprintf(`DELETE FROM %s.silicoid_models WHERE id = ?`, s.dbName)

	opResult := s.readWrite.ExecuteDb(query, id)
	if !opResult.IsSuccess() {
		return fmt.Errorf("硬删除AI模型失败: %v", opResult.Error)
	}

	log.Printf("⚠️ 成功硬删除AI模型: id=%d", id)
	return nil
}

// ==================== 提供商模型表通用 CRUD 操作 ====================

// ProviderModel 提供商模型结构体（适用于所有提供商模型表）
type ProviderModel struct {
	ID              *int     `json:"id,omitempty"`
	ModelCode       string   `json:"model_code"`              // 模型代码(关联ai_models.model_code)
	ModelName       string   `json:"model_name"`
	ModelDisplayName *string `json:"model_display_name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	ContextWindow   *int     `json:"context_window,omitempty"`
	MaxOutputTokens *int     `json:"max_output_tokens,omitempty"`
	InputCostPer1K  *float64 `json:"input_cost_per_1k,omitempty"`
	OutputCostPer1K *float64 `json:"output_cost_per_1k,omitempty"`
	IsAvailable     *int     `json:"is_available,omitempty"` // 1-可用, 0-不可用
	Deprecated      *int     `json:"deprecated,omitempty"`    // 1-已弃用, 0-未弃用
}

// getProviderModelTableName 获取提供商模型表名
// 支持的提供商: Anthropic, OpenAI, DeepSeek, MoonShot, xAI, Meta, DigitalSingularity, Bytedance, Google, Alibaba, OpenRouter
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

// CreateProviderModel 创建单个提供商模型
// 参数:
//   provider: 提供商名称（如 "Anthropic", "OpenAI" 等）
//   model: 模型信息
// 返回: 新创建的模型ID和错误信息
func (s *SilicoidDataService) CreateProviderModel(provider string, model ProviderModel) (int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("创建提供商模型异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return 0, fmt.Errorf("不支持的提供商: %s", provider)
	}

	// 验证必填字段
	if model.ModelName == "" {
		return 0, fmt.Errorf("model_name 不能为空")
	}
	if model.ModelCode == "" {
		return 0, fmt.Errorf("model_code 不能为空")
	}

	// 检查 model_name 是否已存在
	checkQuery := fmt.Sprintf(`SELECT id FROM %s.%s WHERE model_name = ?`, s.dbName, tableName)
	checkResult := s.readWrite.QueryDb(checkQuery, model.ModelName)
	if checkResult.IsSuccess() {
		if rows, ok := checkResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			return 0, fmt.Errorf("model_name %s 已存在", model.ModelName)
		}
	}

	// 构建插入语句
	fields := []string{"model_code", "model_name"}
	placeholders := []string{"?", "?"}
	args := []interface{}{model.ModelCode, model.ModelName}

	if model.ModelDisplayName != nil {
		fields = append(fields, "model_display_name")
		placeholders = append(placeholders, "?")
		args = append(args, *model.ModelDisplayName)
	}
	if model.Description != nil {
		fields = append(fields, "description")
		placeholders = append(placeholders, "?")
		args = append(args, *model.Description)
	}
	if model.ContextWindow != nil {
		fields = append(fields, "context_window")
		placeholders = append(placeholders, "?")
		args = append(args, *model.ContextWindow)
	}
	if model.MaxOutputTokens != nil {
		fields = append(fields, "max_output_tokens")
		placeholders = append(placeholders, "?")
		args = append(args, *model.MaxOutputTokens)
	}
	if model.InputCostPer1K != nil {
		fields = append(fields, "input_cost_per_1k")
		placeholders = append(placeholders, "?")
		args = append(args, *model.InputCostPer1K)
	}
	if model.OutputCostPer1K != nil {
		fields = append(fields, "output_cost_per_1k")
		placeholders = append(placeholders, "?")
		args = append(args, *model.OutputCostPer1K)
	}
	if model.IsAvailable != nil {
		fields = append(fields, "is_available")
		placeholders = append(placeholders, "?")
		args = append(args, *model.IsAvailable)
	}
	if model.Deprecated != nil {
		fields = append(fields, "deprecated")
		placeholders = append(placeholders, "?")
		args = append(args, *model.Deprecated)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s.%s (%s)
		VALUES (%s)
	`, s.dbName, tableName, strings.Join(fields, ", "), strings.Join(placeholders, ", "))

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return 0, fmt.Errorf("创建提供商模型失败: %v", opResult.Error)
	}

	// 获取新创建的ID
	idQuery := fmt.Sprintf(`SELECT id FROM %s.%s WHERE model_name = ? ORDER BY id DESC LIMIT 1`, s.dbName, tableName)
	idResult := s.readWrite.QueryDb(idQuery, model.ModelName)
	if idResult.IsSuccess() {
		if rows, ok := idResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			newID := getIntValue(rows[0]["id"])
			log.Printf("✅ 成功创建提供商模型: provider=%s, id=%d, model_name=%s", provider, newID, model.ModelName)
			return newID, nil
		}
	}

	return 0, fmt.Errorf("创建成功但无法获取新模型ID")
}

// CreateProviderModelsBatch 批量创建提供商模型
// 参数:
//   provider: 提供商名称
//   models: 模型信息列表
// 返回: 成功创建的模型ID列表和错误信息
func (s *SilicoidDataService) CreateProviderModelsBatch(provider string, models []ProviderModel) ([]int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量创建提供商模型异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return nil, fmt.Errorf("不支持的提供商: %s", provider)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("模型列表不能为空")
	}

	ids := make([]int, 0, len(models))
	for _, model := range models {
		id, err := s.CreateProviderModel(provider, model)
		if err != nil {
			return ids, fmt.Errorf("批量创建失败: %v", err)
		}
		ids = append(ids, id)
	}

	log.Printf("✅ 成功批量创建 %d 个提供商模型: provider=%s", len(ids), provider)
	return ids, nil
}

// GetProviderModelByName 根据model_name获取提供商模型
// 参数:
//   provider: 提供商名称
//   modelName: 模型名称
//   includeDisabled: 是否包含已禁用的模型（默认false）
// 返回: 模型信息和错误信息
func (s *SilicoidDataService) GetProviderModelByName(provider string, modelName string, includeDisabled ...bool) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取提供商模型异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return nil, fmt.Errorf("不支持的提供商: %s", provider)
	}

	includeDisabledFlag := false
	if len(includeDisabled) > 0 {
		includeDisabledFlag = includeDisabled[0]
	}

	query := fmt.Sprintf(`
		SELECT id, model_name, model_display_name, description, context_window,
		       max_output_tokens, input_cost_per_1k, output_cost_per_1k,
		       is_available, deprecated, created_at, updated_at
		FROM %s.%s
		WHERE model_name = ?
	`, s.dbName, tableName)

	args := []interface{}{modelName}
	if !includeDisabledFlag {
		query += " AND is_available = 1"
	}

	opResult := s.readWrite.QueryDb(query, args...)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询提供商模型失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 model_name 为 %s 的提供商模型", modelName)
	}

	return rows[0], nil
}

// GetAllProviderModels 获取所有提供商模型（支持过滤）
// 参数:
//   provider: 提供商名称
//   filters: 过滤条件
//     - isAvailable: 是否可用（nil表示不过滤）
//     - deprecated: 是否已弃用（nil表示不过滤）
//   orderBy: 排序字段（默认 "id ASC"）
// 返回: 模型列表和错误信息
func (s *SilicoidDataService) GetAllProviderModels(provider string, filters map[string]interface{}, orderBy ...string) ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取提供商模型列表异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return nil, fmt.Errorf("不支持的提供商: %s", provider)
	}

	// 构建WHERE子句
	whereClause := ""
	args := []interface{}{}

	if isAvailable, ok := filters["is_available"]; ok && isAvailable != nil {
		if isAvail, ok := isAvailable.(int); ok {
			whereClause += " WHERE is_available = ?"
			args = append(args, isAvail)
		}
	}

	if deprecated, ok := filters["deprecated"]; ok && deprecated != nil {
		if dep, ok := deprecated.(int); ok {
			if whereClause == "" {
				whereClause = " WHERE deprecated = ?"
			} else {
				whereClause += " AND deprecated = ?"
			}
			args = append(args, dep)
		}
	}

	// 设置默认排序
	sortBy := "id ASC"
	if len(orderBy) > 0 && orderBy[0] != "" {
		sortBy = orderBy[0]
	}

	query := fmt.Sprintf(`
		SELECT id, model_code, model_name, model_display_name, description, context_window,
		       max_output_tokens, input_cost_per_1k, output_cost_per_1k,
		       is_available, deprecated, created_at, updated_at
		FROM %s.%s
		%s
		ORDER BY %s
	`, s.dbName, tableName, whereClause, sortBy)

	var opResult *datahandle.OperationResult
	if len(args) > 0 {
		opResult = s.readWrite.QueryDb(query, args...)
	} else {
		opResult = s.readWrite.QueryDb(query)
	}

	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询提供商模型列表失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查询结果格式错误")
	}

	return rows, nil
}

// UpdateProviderModel 更新提供商模型
// 参数:
//   provider: 提供商名称
//   code: 模型代码 (model_code)
//   model: 要更新的模型信息（只更新非nil字段）
// 返回: 错误信息
func (s *SilicoidDataService) UpdateProviderModel(provider string, code string, model ProviderModel) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("更新提供商模型异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return fmt.Errorf("不支持的提供商: %s", provider)
	}

	if code == "" {
		return fmt.Errorf("model_code 不能为空")
	}

	// 检查模型是否存在（通过 model_code）
	checkQuery := fmt.Sprintf(`SELECT id FROM %s.%s WHERE model_code = ?`, s.dbName, tableName)
	checkResult := s.readWrite.QueryDb(checkQuery, code)
	if !checkResult.IsSuccess() {
		return fmt.Errorf("查询模型失败: %v", checkResult.Error)
	}
	if rows, ok := checkResult.Data.([]map[string]interface{}); !ok || len(rows) == 0 {
		return fmt.Errorf("模型不存在: model_code=%s", code)
	}

	// 构建更新字段
	setClause := []string{}
	args := []interface{}{}

	if model.ModelName != "" {
		// 检查新的 model_name 是否与其他记录冲突
		checkQuery := fmt.Sprintf(`SELECT id FROM %s.%s WHERE model_name = ? AND model_code != ?`, s.dbName, tableName)
		checkResult := s.readWrite.QueryDb(checkQuery, model.ModelName, code)
		if checkResult.IsSuccess() {
			if rows, ok := checkResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
				return fmt.Errorf("model_name %s 已被其他记录使用", model.ModelName)
			}
		}
		setClause = append(setClause, "model_name = ?")
		args = append(args, model.ModelName)
	}

	if model.ModelDisplayName != nil {
		setClause = append(setClause, "model_display_name = ?")
		args = append(args, *model.ModelDisplayName)
	}
	if model.Description != nil {
		setClause = append(setClause, "description = ?")
		args = append(args, *model.Description)
	}
	if model.ContextWindow != nil {
		setClause = append(setClause, "context_window = ?")
		args = append(args, *model.ContextWindow)
	}
	if model.MaxOutputTokens != nil {
		setClause = append(setClause, "max_output_tokens = ?")
		args = append(args, *model.MaxOutputTokens)
	}
	if model.InputCostPer1K != nil {
		setClause = append(setClause, "input_cost_per_1k = ?")
		args = append(args, *model.InputCostPer1K)
	}
	if model.OutputCostPer1K != nil {
		setClause = append(setClause, "output_cost_per_1k = ?")
		args = append(args, *model.OutputCostPer1K)
	}
	if model.IsAvailable != nil {
		setClause = append(setClause, "is_available = ?")
		args = append(args, *model.IsAvailable)
	}
	if model.Deprecated != nil {
		setClause = append(setClause, "deprecated = ?")
		args = append(args, *model.Deprecated)
	}

	if len(setClause) == 0 {
		return fmt.Errorf("没有有效的更新字段")
	}

	// 自动更新 updated_at 字段
	setClause = append(setClause, "updated_at = NOW()")

	// 添加 model_code 参数
	args = append(args, code)

	// 构建完整的 SET 子句
	setClauseStr := setClause[0]
	for i := 1; i < len(setClause); i++ {
		setClauseStr += ", " + setClause[i]
	}

	query := fmt.Sprintf(`
		UPDATE %s.%s
		SET %s
		WHERE model_code = ?
	`, s.dbName, tableName, setClauseStr)

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("更新提供商模型失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功更新提供商模型: provider=%s, model_code=%s", provider, code)
	return nil
}

// ProviderModelUpdate 提供商模型更新结构体
type ProviderModelUpdate struct {
	Code  string
	Model ProviderModel
}

// UpdateProviderModelsBatch 批量更新提供商模型
// 参数:
//   provider: 提供商名称
//   updates: 更新列表，每个元素包含 code 和要更新的模型信息
// 返回: 错误信息
func (s *SilicoidDataService) UpdateProviderModelsBatch(provider string, updates []ProviderModelUpdate) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量更新提供商模型异常: %v", r)
		}
	}()

	if len(updates) == 0 {
		return fmt.Errorf("更新列表不能为空")
	}

	for _, update := range updates {
		if err := s.UpdateProviderModel(provider, update.Code, update.Model); err != nil {
			return fmt.Errorf("批量更新失败，model_code=%s: %v", update.Code, err)
		}
	}

	log.Printf("✅ 成功批量更新 %d 个提供商模型: provider=%s", len(updates), provider)
	return nil
}

// DeleteProviderModel 软删除提供商模型（设置 is_available = 0）
// 参数:
//   provider: 提供商名称
//   code: 模型代码 (model_code)
// 返回: 错误信息
func (s *SilicoidDataService) DeleteProviderModel(provider string, code string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("删除提供商模型异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return fmt.Errorf("不支持的提供商: %s", provider)
	}

	if code == "" {
		return fmt.Errorf("model_code 不能为空")
	}

	// 检查模型是否存在（通过 model_code）
	checkQuery := fmt.Sprintf(`SELECT id FROM %s.%s WHERE model_code = ?`, s.dbName, tableName)
	checkResult := s.readWrite.QueryDb(checkQuery, code)
	if !checkResult.IsSuccess() {
		return fmt.Errorf("查询模型失败: %v", checkResult.Error)
	}
	if rows, ok := checkResult.Data.([]map[string]interface{}); !ok || len(rows) == 0 {
		return fmt.Errorf("模型不存在: model_code=%s", code)
	}

	query := fmt.Sprintf(`UPDATE %s.%s SET is_available = 0 WHERE model_code = ?`, s.dbName, tableName)
	opResult := s.readWrite.ExecuteDb(query, code)
	if !opResult.IsSuccess() {
		return fmt.Errorf("软删除提供商模型失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功软删除提供商模型: provider=%s, model_code=%s", provider, code)
	return nil
}

// DeleteProviderModelsBatch 批量软删除提供商模型
// 参数:
//   provider: 提供商名称
//   codes: 模型代码列表 (model_code)
// 返回: 错误信息
func (s *SilicoidDataService) DeleteProviderModelsBatch(provider string, codes []string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量删除提供商模型异常: %v", r)
		}
	}()

	if len(codes) == 0 {
		return fmt.Errorf("model_code列表不能为空")
	}

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return fmt.Errorf("不支持的提供商: %s", provider)
	}

	// 构建批量更新语句
	placeholders := make([]string, len(codes))
	args := make([]interface{}, len(codes))
	for i, code := range codes {
		placeholders[i] = "?"
		args[i] = code
	}

	query := fmt.Sprintf(`UPDATE %s.%s SET is_available = 0 WHERE model_code IN (%s)`, s.dbName, tableName, strings.Join(placeholders, ", "))

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("批量软删除提供商模型失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功批量软删除 %d 个提供商模型: provider=%s", len(codes), provider)
	return nil
}

// HardDeleteProviderModel 硬删除提供商模型（物理删除）
// 参数:
//   provider: 提供商名称
//   code: 模型代码 (model_code)
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteProviderModel(provider string, code string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("硬删除提供商模型异常: %v", r)
		}
	}()

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return fmt.Errorf("不支持的提供商: %s", provider)
	}

	if code == "" {
		return fmt.Errorf("model_code 不能为空")
	}

	// 检查模型是否存在（通过 model_code）
	checkQuery := fmt.Sprintf(`SELECT id FROM %s.%s WHERE model_code = ?`, s.dbName, tableName)
	checkResult := s.readWrite.QueryDb(checkQuery, code)
	if !checkResult.IsSuccess() {
		return fmt.Errorf("查询模型失败: %v", checkResult.Error)
	}
	if rows, ok := checkResult.Data.([]map[string]interface{}); !ok || len(rows) == 0 {
		return fmt.Errorf("模型不存在: model_code=%s", code)
	}

	query := fmt.Sprintf(`DELETE FROM %s.%s WHERE model_code = ?`, s.dbName, tableName)
	opResult := s.readWrite.ExecuteDb(query, code)
	if !opResult.IsSuccess() {
		return fmt.Errorf("硬删除提供商模型失败: %v", opResult.Error)
	}

	log.Printf("⚠️ 成功硬删除提供商模型: provider=%s, model_code=%s", provider, code)
	return nil
}

// HardDeleteProviderModelsBatch 批量硬删除提供商模型
// 参数:
//   provider: 提供商名称
//   codes: 模型代码列表 (model_code)
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteProviderModelsBatch(provider string, codes []string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量硬删除提供商模型异常: %v", r)
		}
	}()

	if len(codes) == 0 {
		return fmt.Errorf("model_code列表不能为空")
	}

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return fmt.Errorf("不支持的提供商: %s", provider)
	}

	// 构建批量删除语句
	placeholders := make([]string, len(codes))
	args := make([]interface{}, len(codes))
	for i, code := range codes {
		placeholders[i] = "?"
		args[i] = code
	}

	query := fmt.Sprintf(`DELETE FROM %s.%s WHERE model_code IN (%s)`, s.dbName, tableName, strings.Join(placeholders, ", "))

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("批量硬删除提供商模型失败: %v", opResult.Error)
	}

	log.Printf("⚠️ 成功批量硬删除 %d 个提供商模型: provider=%s", len(codes), provider)
	return nil
}

// HardDeleteProviderModelsByNames 批量硬删除提供商模型（按 model_name）
// 参数:
//   provider: 提供商名称
//   modelNames: 模型名称列表 (model_name)
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteProviderModelsByNames(provider string, modelNames []string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量硬删除提供商模型异常: %v", r)
		}
	}()

	if len(modelNames) == 0 {
		return fmt.Errorf("model_name列表不能为空")
	}

	tableName := getProviderModelTableName(provider)
	if tableName == "" {
		return fmt.Errorf("不支持的提供商: %s", provider)
	}

	// 构建批量删除语句
	placeholders := make([]string, len(modelNames))
	args := make([]interface{}, len(modelNames))
	for i, name := range modelNames {
		placeholders[i] = "?"
		args[i] = name
	}

	query := fmt.Sprintf(`DELETE FROM %s.%s WHERE model_name IN (%s)`, s.dbName, tableName, strings.Join(placeholders, ", "))

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("批量硬删除提供商模型失败: %v", opResult.Error)
	}

	log.Printf("⚠️ 成功批量硬删除 %d 个提供商模型: provider=%s", len(modelNames), provider)
	return nil
}

