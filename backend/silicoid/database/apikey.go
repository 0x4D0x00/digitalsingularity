package database

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
)

// GetAllModelApiKeys 获取所有模型API密钥列表（支持过滤条件）
// 参数:
//   modelCode: 模型代码过滤，空字符串表示不过滤
//   status: 状态过滤，nil表示不过滤
//   orderBy: 排序字段，默认 'priority DESC, id ASC'
// 返回: 密钥信息列表和错误信息
func (s *SilicoidDataService) GetAllModelApiKeys(
	modelCode string,
	status *int,
	orderBy string,
) ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取模型API密钥列表异常: %v", r)
		}
	}()

	// 构建WHERE条件
	whereConditions := []string{}
	args := []interface{}{}

	if modelCode != "" {
		whereConditions = append(whereConditions, "k.model_code = ?")
		args = append(args, modelCode)
	}

	if status != nil {
		whereConditions = append(whereConditions, "k.status = ?")
		args = append(args, *status)
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
		orderBy = "k.priority DESC, k.id ASC"
	}

	query := fmt.Sprintf(`
		SELECT k.id, k.model_code, k.api_key, k.key_name, k.status, k.priority,
		       k.usage_count, k.success_count, k.fail_count,
		       k.last_used_at, k.last_success_at, k.last_fail_at, k.fail_reason,
		       k.rate_limit_per_min, k.rate_limit_per_day, k.expires_at,
		       k.created_at, k.updated_at,
		       m.provider, m.model_type
		FROM %s.models_api_keys k
		LEFT JOIN %s.silicoid_models m ON k.model_code = m.model_code
		%s
		ORDER BY %s
	`, s.dbName, s.dbName, whereClause, orderBy)

	var opResult *datahandle.OperationResult
	if len(args) > 0 {
		opResult = s.readWrite.QueryDb(query, args...)
	} else {
		opResult = s.readWrite.QueryDb(query)
	}

	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询模型API密钥列表失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查询结果格式错误")
	}

	return rows, nil
}

// GetModelAPIKeys 根据模型代码获取模型API密钥列表
// 先尝试从Redis缓存读取，如果没有则从数据库查询并缓存到Redis
func (s *SilicoidDataService) GetModelAPIKeys(modelCode string) ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取模型API密钥异常: %v", r)
		}
	}()

	// 第一层: 尝试从Redis缓存获取
	cacheKey := fmt.Sprintf("model:apikeys:raw:%s", modelCode)
	result := s.readWrite.GetRedis(cacheKey)
	
	if result.IsSuccess() {
		jsonStr, ok := result.Data.(string)
		if ok && jsonStr != "" {
			var rows []map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &rows); err == nil {
				log.Printf("✅ 从Redis缓存获取模型API密钥列表: modelCode=%s, 数量=%d", modelCode, len(rows))
				return rows, nil
			}
		}
	}

	// 第二层: 从数据库查询
	query := fmt.Sprintf(`
		SELECT k.id, k.model_code, k.api_key, k.key_name, k.status, k.priority,
		       k.usage_count, k.success_count, k.fail_count,
		       k.last_used_at, k.last_success_at, k.last_fail_at, k.fail_reason,
		       k.rate_limit_per_min, k.rate_limit_per_day,
		       m.provider, m.model_type
		FROM %s.models_api_keys k
		JOIN %s.silicoid_models m ON k.model_code = m.model_code
		WHERE k.model_code = ? AND k.status = 1 AND m.status = 1
		ORDER BY k.priority DESC, k.success_count DESC
	`, s.dbName, s.dbName)

	opResult := s.readWrite.QueryDb(query, modelCode)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询API密钥失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查询结果格式错误")
	}

	// 缓存到Redis（设置过期时间为1小时）
	if len(rows) > 0 {
		jsonData, err := json.Marshal(rows)
		if err == nil {
			// 缓存1小时
			cacheResult := s.readWrite.SetRedis(cacheKey, string(jsonData), time.Hour)
			if !cacheResult.IsSuccess() {
				// 静默失败，不影响主流程
				if cacheResult.Error != nil && cacheResult.Error.Error() != "READONLY You can't write against a read only replica." {
					log.Printf("缓存模型API密钥到Redis失败: %v", cacheResult.Error)
				}
			} else {
				log.Printf("✅ 成功缓存模型API密钥列表到Redis: modelCode=%s, 数量=%d", modelCode, len(rows))
			}
		}
	}

	return rows, nil
}

// GetAPIKeyByModelCode 根据 model_code 获取可用的 API Key
// 先尝试从Redis缓存读取，如果没有则从数据库查询并缓存到Redis
func (s *SilicoidDataService) GetAPIKeyByModelCode(modelCode string) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取 API Key 异常: %v", r)
		}
	}()

	// 第一层: 尝试从Redis缓存获取
	cacheKey := fmt.Sprintf("model:apikey:bycode:%s", modelCode)
	result := s.readWrite.GetRedis(cacheKey)
	
	if result.IsSuccess() {
		jsonStr, ok := result.Data.(string)
		if ok && jsonStr != "" {
			var row map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &row); err == nil {
				log.Printf("✅ 从Redis缓存获取 API Key: modelCode=%s", modelCode)
				return row, nil
			}
		}
	}

	// 第二层: 从数据库查询
	query := fmt.Sprintf(`
		SELECT k.id, k.model_code, k.api_key, k.key_name, k.status, k.priority,
		       m.provider, m.model_type
		FROM %s.models_api_keys k
		INNER JOIN %s.silicoid_models m ON k.model_code = m.model_code
		WHERE k.model_code = ? AND k.status = 1 AND m.status = 1
		ORDER BY k.priority ASC, k.id ASC
		LIMIT 1
	`, s.dbName, s.dbName)

	opResult := s.readWrite.QueryDb(query, modelCode)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询 API Key 失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 model_code=%s 可用的 API Key", modelCode)
	}

	row := rows[0]

	// 缓存到Redis（设置过期时间为1小时）
	jsonData, err := json.Marshal(row)
	if err == nil {
		cacheResult := s.readWrite.SetRedis(cacheKey, string(jsonData), time.Hour)
		if !cacheResult.IsSuccess() {
			// 静默失败，不影响主流程
			if cacheResult.Error != nil && cacheResult.Error.Error() != "READONLY You can't write against a read only replica." {
				log.Printf("缓存 API Key 到Redis失败: %v", cacheResult.Error)
			}
		} else {
			log.Printf("✅ 成功缓存 API Key 到Redis: modelCode=%s", modelCode)
		}
	}

	return row, nil
}

// ==================== Model API Keys CRUD 操作 ====================

// clearModelApiKeysCache 清除指定模型的API密钥Redis缓存
// 参数:
//   modelCode: 模型代码（必填）
func (s *SilicoidDataService) clearModelApiKeysCache(modelCode string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("清除API密钥缓存异常: %v", r)
		}
	}()

	if modelCode == "" {
		return
	}

	// 清除 ModelManager 使用的缓存 (model:apikeys:{modelCode})
	cacheKey := fmt.Sprintf("model:apikeys:%s", modelCode)
	result := s.readWrite.DeleteRedis(cacheKey)
	if !result.IsSuccess() {
		// 检查是否为Redis只读错误，如果是则静默跳过
		if result.Error != nil && result.Error.Error() == "READONLY You can't write against a read only replica." {
			// 静默跳过只读副本错误
		} else {
			log.Printf("清除API密钥缓存失败 (modelCode=%s): %v", modelCode, result.Error)
		}
	} else {
		log.Printf("✅ 成功清除API密钥缓存: modelCode=%s", modelCode)
	}

	// 清除 GetAPIKeyByModelCode 使用的缓存 (model:apikey:bycode:{modelCode})
	cacheKeyByCode := fmt.Sprintf("model:apikey:bycode:%s", modelCode)
	s.readWrite.DeleteRedis(cacheKeyByCode)

	// 清除原始数据缓存 (model:apikeys:raw:{modelCode})
	cacheKeyRaw := fmt.Sprintf("model:apikeys:raw:%s", modelCode)
	resultRaw := s.readWrite.DeleteRedis(cacheKeyRaw)
	if !resultRaw.IsSuccess() {
		if resultRaw.Error != nil && resultRaw.Error.Error() == "READONLY You can't write against a read only replica." {
			// 静默跳过只读副本错误
		} else {
			log.Printf("清除原始API密钥缓存失败 (modelCode=%s): %v", modelCode, resultRaw.Error)
		}
	} else {
		log.Printf("✅ 成功清除原始API密钥缓存: modelCode=%s", modelCode)
	}
}

// CreateModelApiKey 创建新的模型API密钥
// 参数:
//   modelCode: 关联的模型代码（必填）
//   apiKey: API密钥（必填）
//   keyName: 密钥名称/备注（可选）
//   status: 状态，1-有效, 0-无效, 2-待验证（默认 1）
//   priority: 优先级，数值越大优先级越高（默认 0）
//   rateLimitPerMin: 每分钟速率限制（可选）
//   rateLimitPerDay: 每天速率限制（可选）
//   expiresAt: 过期时间（可选）
// 返回: 新创建的密钥ID和错误信息
func (s *SilicoidDataService) CreateModelApiKey(
	modelCode string,
	apiKey string,
	keyName *string,
	status *int,
	priority *int,
	rateLimitPerMin *int,
	rateLimitPerDay *int,
	expiresAt *string,
) (int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("创建模型API密钥异常: %v", r)
		}
	}()

	// 验证必填字段
	if modelCode == "" {
		return 0, fmt.Errorf("model_code 不能为空")
	}
	if apiKey == "" {
		return 0, fmt.Errorf("api_key 不能为空")
	}

	// 检查模型是否存在
	_, err := s.GetAIModelByCode(modelCode, false)
	if err != nil {
		return 0, fmt.Errorf("模型不存在: %v", err)
	}

	// 检查 model_code 和 api_key 的组合是否已存在（唯一约束）
	checkQuery := fmt.Sprintf(`SELECT id FROM %s.models_api_keys WHERE model_code = ? AND api_key = ?`, s.dbName)
	checkResult := s.readWrite.QueryDb(checkQuery, modelCode, apiKey)
	if checkResult.IsSuccess() {
		if rows, ok := checkResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			return 0, fmt.Errorf("该模型已存在相同的 API Key")
		}
	}

	// 设置默认值
	if status == nil {
		defaultStatus := 1
		status = &defaultStatus
	}
	if priority == nil {
		defaultPriority := 0
		priority = &defaultPriority
	}

	// 构建插入SQL
	query := fmt.Sprintf(`
		INSERT INTO %s.models_api_keys 
		(model_code, api_key, key_name, status, priority, rate_limit_per_min, 
		 rate_limit_per_day, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.dbName)

	// 处理 expires_at
	var expiresAtValue interface{}
	if expiresAt != nil && *expiresAt != "" {
		expiresAtValue = *expiresAt
	} else {
		expiresAtValue = nil
	}

	// 执行插入
	opResult := s.readWrite.ExecuteDb(query,
		modelCode, apiKey, keyName, *status, *priority,
		rateLimitPerMin, rateLimitPerDay, expiresAtValue,
	)

	if !opResult.IsSuccess() {
		return 0, fmt.Errorf("创建模型API密钥失败: %v", opResult.Error)
	}

	// 获取插入的ID
	idQuery := fmt.Sprintf(`SELECT id FROM %s.models_api_keys WHERE model_code = ? AND api_key = ? ORDER BY id DESC LIMIT 1`, s.dbName)
	idResult := s.readWrite.QueryDb(idQuery, modelCode, apiKey)
	if idResult.IsSuccess() {
		if rows, ok := idResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			keyID := getIntValue(rows[0]["id"])
			log.Printf("✅ 成功创建模型API密钥: model_code=%s, id=%d", modelCode, keyID)
			
			// 清除Redis缓存，让ModelManager重新加载
			s.clearModelApiKeysCache(modelCode)
			
			return keyID, nil
		}
	}

	return 0, fmt.Errorf("创建成功但无法获取密钥ID")
}

// GetModelApiKeyByID 根据ID获取模型API密钥详细信息
// 参数:
//   id: 密钥ID
// 返回: 密钥信息映射和错误信息
func (s *SilicoidDataService) GetModelApiKeyByID(id int) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取模型API密钥异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT k.id, k.model_code, k.api_key, k.key_name, k.status, k.priority,
		       k.usage_count, k.success_count, k.fail_count,
		       k.last_used_at, k.last_success_at, k.last_fail_at, k.fail_reason,
		       k.rate_limit_per_min, k.rate_limit_per_day, k.expires_at,
		       k.created_at, k.updated_at,
		       m.provider, m.model_type
		FROM %s.models_api_keys k
		LEFT JOIN %s.silicoid_models m ON k.model_code = m.model_code
		WHERE k.id = ?
	`, s.dbName, s.dbName)

	opResult := s.readWrite.QueryDb(query, id)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询模型API密钥失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 id=%d 的模型API密钥", id)
	}

	return rows[0], nil
}

// UpdateModelApiKey 更新模型API密钥信息
// 参数:
//   id: 密钥ID
//   updates: 要更新的字段映射，可以包含以下字段：
//     - api_key, key_name, status, priority,
//       rate_limit_per_min, rate_limit_per_day, expires_at
// 返回: 错误信息
func (s *SilicoidDataService) UpdateModelApiKey(id int, updates map[string]interface{}) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("更新模型API密钥异常: %v", r)
		}
	}()

	if len(updates) == 0 {
		return fmt.Errorf("没有要更新的字段")
	}

	// 检查密钥是否存在
	_, err := s.GetModelApiKeyByID(id)
	if err != nil {
		return fmt.Errorf("密钥不存在: %v", err)
	}

	// 允许更新的字段列表
	allowedFields := map[string]bool{
		"api_key":            true,
		"key_name":           true,
		"status":             true,
		"priority":           true,
		"rate_limit_per_min": true,
		"rate_limit_per_day": true,
		"expires_at":         true,
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
		UPDATE %s.models_api_keys
		SET %s
		WHERE id = ?
	`, s.dbName, setClauseStr)

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("更新模型API密钥失败: %v", opResult.Error)
	}

	// 获取 model_code 以便清除缓存
	keyInfo, err := s.GetModelApiKeyByID(id)
	if err == nil {
		modelCode := getStringValue(keyInfo["model_code"])
		// 清除Redis缓存，让ModelManager重新加载
		s.clearModelApiKeysCache(modelCode)
	}

	log.Printf("✅ 成功更新模型API密钥: id=%d", id)
	return nil
}

// UpdateModelApiKeyUsage 更新API密钥的使用统计信息
// 参数:
//   id: 密钥ID
//   success: 是否成功（true-成功，false-失败）
//   failReason: 失败原因（可选，仅在失败时使用）
// 返回: 错误信息
func (s *SilicoidDataService) UpdateModelApiKeyUsage(id int, success bool, failReason *string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("更新API密钥使用统计异常: %v", r)
		}
	}()

	// 检查密钥是否存在
	_, err := s.GetModelApiKeyByID(id)
	if err != nil {
		return fmt.Errorf("密钥不存在: %v", err)
	}

	var query string
	var args []interface{}

	if success {
		// 成功：增加 usage_count 和 success_count，更新 last_used_at 和 last_success_at
		query = fmt.Sprintf(`
			UPDATE %s.models_api_keys
			SET usage_count = usage_count + 1,
			    success_count = success_count + 1,
			    last_used_at = NOW(),
			    last_success_at = NOW()
			WHERE id = ?
		`, s.dbName)
		args = []interface{}{id}
	} else {
		// 失败：增加 usage_count 和 fail_count，更新 last_used_at 和 last_fail_at
		query = fmt.Sprintf(`
			UPDATE %s.models_api_keys
			SET usage_count = usage_count + 1,
			    fail_count = fail_count + 1,
			    last_used_at = NOW(),
			    last_fail_at = NOW(),
			    fail_reason = ?
			WHERE id = ?
		`, s.dbName)
		if failReason != nil {
			args = []interface{}{*failReason, id}
		} else {
			args = []interface{}{nil, id}
		}
	}

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("更新API密钥使用统计失败: %v", opResult.Error)
	}

	// 注意：使用统计更新不需要清除缓存，因为不影响密钥的可用性
	// 但如果需要实时反映统计信息，可以取消下面的注释
	// keyInfo, err := s.GetModelApiKeyByID(id)
	// if err == nil {
	// 	modelID := getIntValue(keyInfo["model_id"])
	// 	s.clearModelApiKeysCache("", modelID)
	// }

	return nil
}

// DeleteModelApiKey 软删除模型API密钥（设置status=0）
// 参数:
//   id: 密钥ID
// 返回: 错误信息
func (s *SilicoidDataService) DeleteModelApiKey(id int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("删除模型API密钥异常: %v", r)
		}
	}()

	// 检查密钥是否存在并获取 model_code
	keyInfo, err := s.GetModelApiKeyByID(id)
	if err != nil {
		return fmt.Errorf("密钥不存在: %v", err)
	}
	modelCode := getStringValue(keyInfo["model_code"])

	query := fmt.Sprintf(`
		UPDATE %s.models_api_keys
		SET status = 0
		WHERE id = ?
	`, s.dbName)

	opResult := s.readWrite.ExecuteDb(query, id)
	if !opResult.IsSuccess() {
		return fmt.Errorf("删除模型API密钥失败: %v", opResult.Error)
	}

	// 清除Redis缓存，让ModelManager重新加载
	s.clearModelApiKeysCache(modelCode)

	log.Printf("✅ 成功软删除模型API密钥: id=%d", id)
	return nil
}

// HardDeleteModelApiKey 硬删除模型API密钥（从数据库中物理删除）
// 警告：此操作不可恢复，请谨慎使用
// 参数:
//   id: 密钥ID
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteModelApiKey(id int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("硬删除模型API密钥异常: %v", r)
		}
	}()

	// 检查密钥是否存在并获取 model_code
	keyInfo, err := s.GetModelApiKeyByID(id)
	if err != nil {
		return fmt.Errorf("密钥不存在: %v", err)
	}
	modelCode := getStringValue(keyInfo["model_code"])

	query := fmt.Sprintf(`DELETE FROM %s.models_api_keys WHERE id = ?`, s.dbName)

	opResult := s.readWrite.ExecuteDb(query, id)
	if !opResult.IsSuccess() {
		return fmt.Errorf("硬删除模型API密钥失败: %v", opResult.Error)
	}

	// 清除Redis缓存，让ModelManager重新加载
	s.clearModelApiKeysCache(modelCode)

	log.Printf("⚠️ 成功硬删除模型API密钥: id=%d", id)
	return nil
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
	if i, ok := value.(int64); ok {
		return int(i)
	}
	if i, ok := value.(int); ok {
		return i
	}
	return 0
}