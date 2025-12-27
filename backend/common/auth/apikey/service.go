package apikey

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"digitalsingularity/backend/common/configs/settings"
	"digitalsingularity/backend/common/utils/datahandle"

	_ "github.com/go-sql-driver/mysql"
)

// 创建logger
var logger = log.New(log.Writer(), "[ApiKey] ", log.LstdFlags)

// ApiKeyService API密钥服务，处理API密钥验证和用户令牌余额检查
type ApiKeyService struct {
	readWrite *datahandle.CommonReadWriteService
	dbConfig  map[string]interface{}
}

// NewApiKeyService 创建新的ApiKeyService实例
func NewApiKeyService() *ApiKeyService {
	// 获取设置
	settingsService := settings.NewCommonSettings()
	
	// 创建服务实例
	service := &ApiKeyService{}
	
	// 使用settings中的数据库配置
	service.dbConfig = map[string]interface{}{
		"host":     settingsService.DbHost,
		"port":     settingsService.DbPort,
		"user":     settingsService.DbUser,
		"password": settingsService.DbPassword,
		"database": settingsService.DbNameSilicoid, // 使用silicoid数据库
	}
	
	// 创建读写服务
	readWrite, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		logger.Printf("创建读写服务失败: %v", err)
	}
	service.readWrite = readWrite
	
	return service
}

// NewApiKeyServiceWithDB 使用指定的readWrite服务创建新的ApiKeyService实例
func NewApiKeyServiceWithDB(readWrite *datahandle.CommonReadWriteService) *ApiKeyService {
	// 获取设置
	settingsService := settings.NewCommonSettings()
	
	// 创建服务实例
	service := &ApiKeyService{
		readWrite: readWrite,
	}
	
	// 使用settings中的数据库配置
	service.dbConfig = map[string]interface{}{
		"host":     settingsService.DbHost,
		"port":     settingsService.DbPort,
		"user":     settingsService.DbUser,
		"password": settingsService.DbPassword,
		"database": settingsService.DbNameSilicoid, // 使用silicoid数据库
	}
	
	return service
}

// VerifyApiKey 验证API密钥是否有效
// 返回：是否有效，用户ID，错误消息
func (s *ApiKeyService) VerifyApiKey(apiKey string) (bool, string, error) {
	if apiKey == "" {
		return false, "", fmt.Errorf("API密钥不能为空")
	}

	// 连接数据库
	db, err := s.getDbConnection()
	if err != nil {
		logger.Printf("数据库连接失败: %v", err)
		return false, "", fmt.Errorf("数据库连接失败: %v", err)
	}
	defer db.Close()

	// 查询API密钥
	var id string
	var userId string
	var status int
	var expiresAt sql.NullTime

	query := `
		SELECT id, user_id, status, expires_at
		FROM aibasicplatform.aibasicplatform_user_api_keys
		WHERE api_key = ?
	`
	err = db.QueryRow(query, apiKey).Scan(&id, &userId, &status, &expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, "", fmt.Errorf("无效的API密钥")
		}
		logger.Printf("查询API密钥失败: %v", err)
		return false, "", fmt.Errorf("查询失败: %v", err)
	}

	// 更新最后使用时间
	updateQuery := `
		UPDATE aibasicplatform.aibasicplatform_user_api_keys
		SET last_used_at = NOW()
		WHERE id = ?
	`
	_, err = db.Exec(updateQuery, id)
	if err != nil {
		logger.Printf("更新API密钥使用时间失败: %v", err)
	}

	// 验证状态
	if status != 1 {
		return false, "", fmt.Errorf("API密钥已禁用")
	}

	// 检查是否过期
	now := time.Now()
	if expiresAt.Valid && expiresAt.Time.Before(now) {
		return false, "", fmt.Errorf("API密钥已过期")
	}

	return true, userId, nil
}

// CheckUserTokens 检查用户的令牌余额
// 返回：是否有足够令牌，令牌总数，错误消息，令牌详情
func (s *ApiKeyService) CheckUserTokens(userId string) (bool, int, error, map[string]int) {
	if userId == "" {
		return false, 0, fmt.Errorf("用户ID不能为空"), nil
	}

	// 连接数据库
	db, err := s.getDbConnection()
	if err != nil {
		logger.Printf("数据库连接失败: %v", err)
		return false, 0, fmt.Errorf("数据库连接失败: %v", err), nil
	}
	defer db.Close()

	// 查询用户资产
	var giftedTokens, ownedTokens int
	query := `
		SELECT gifted_tokens, owned_tokens
		FROM aibasicplatform.aibasicplatform_user_assets
		WHERE user_id = ?
	`
	err = db.QueryRow(query, userId).Scan(&giftedTokens, &ownedTokens)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0, fmt.Errorf("用户资产记录不存在"), nil
		}
		logger.Printf("查询用户资产失败: %v", err)
		return false, 0, fmt.Errorf("查询失败: %v", err), nil
	}

	// 计算总令牌数
	totalTokens := giftedTokens + ownedTokens

	// 判断是否有足够的令牌
	hasEnough := totalTokens > 0

	// 构建详细令牌信息
	tokenDetails := map[string]int{
		"gifted_tokens": giftedTokens,
		"owned_tokens":  ownedTokens,
	}

	return hasEnough, totalTokens, nil, tokenDetails
}

// DeductTokens 扣除用户令牌
// 返回：扣除成功，剩余令牌，错误消息
func (s *ApiKeyService) DeductTokens(userId string, tokensToDeduct int) (bool, int, error) {
	if userId == "" {
		return false, 0, fmt.Errorf("用户ID不能为空")
	}

	// 连接数据库
	db, err := s.getDbConnection()
	if err != nil {
		logger.Printf("数据库连接失败: %v", err)
		return false, 0, fmt.Errorf("数据库连接失败: %v", err)
	}
	defer db.Close()

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		logger.Printf("开始事务失败: %v", err)
		return false, 0, fmt.Errorf("开始事务失败: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// 查询用户资产并加锁
	var giftedTokens, ownedTokens int
	query := `
		SELECT gifted_tokens, owned_tokens
		FROM aibasicplatform.aibasicplatform_user_assets
		WHERE user_id = ?
		FOR UPDATE
	`
	err = tx.QueryRow(query, userId).Scan(&giftedTokens, &ownedTokens)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0, fmt.Errorf("用户资产记录不存在")
		}
		logger.Printf("查询用户资产失败: %v", err)
		return false, 0, fmt.Errorf("查询失败: %v", err)
	}

	// 计算总令牌数
	totalTokens := giftedTokens + ownedTokens

	if totalTokens < tokensToDeduct {
		return false, totalTokens, fmt.Errorf("令牌余额不足")
	}

	// 优先扣除赠送的令牌
	deductFromGifted := min(giftedTokens, tokensToDeduct)
	newGiftedTokens := giftedTokens - deductFromGifted

	// 如果赠送的令牌不足，再扣除拥有的令牌
	deductFromOwned := tokensToDeduct - deductFromGifted
	newOwnedTokens := ownedTokens - deductFromOwned

	// 更新用户资产
	updateQuery := `
		UPDATE aibasicplatform.aibasicplatform_user_assets
		SET gifted_tokens = ?, owned_tokens = ?
		WHERE user_id = ?
	`
	_, err = tx.Exec(updateQuery, newGiftedTokens, newOwnedTokens, userId)
	if err != nil {
		logger.Printf("更新用户资产失败: %v", err)
		return false, 0, fmt.Errorf("更新失败: %v", err)
	}

	// 提交事务
	err = tx.Commit()
	if err != nil {
		logger.Printf("提交事务失败: %v", err)
		return false, 0, fmt.Errorf("提交事务失败: %v", err)
	}

	return true, newGiftedTokens + newOwnedTokens, nil
}

// CreateApiKey 为用户创建新的API密钥
// 返回：创建成功，密钥信息，错误消息
func (s *ApiKeyService) CreateApiKey(userId string, description string, expiresInDays int) (bool, map[string]interface{}, error) {
	if userId == "" {
		return false, nil, fmt.Errorf("用户ID不能为空")
	}

	// 使用默认值
	if description == "" {
		now := time.Now().Format("2006-01-02")
		description = fmt.Sprintf("API密钥 - %s", now)
	}
	if expiresInDays == 0 {
		expiresInDays = 365 // 默认一年
	}

	// 连接数据库
	db, err := s.getDbConnection()
	if err != nil {
		logger.Printf("数据库连接失败: %v", err)
		return false, nil, fmt.Errorf("数据库连接失败: %v", err)
	}
	defer db.Close()

	// 检查用户是否存在
	var count int
	checkQuery := `
		SELECT COUNT(*) as count
		FROM common.users
		WHERE user_id = ?
	`
	err = db.QueryRow(checkQuery, userId).Scan(&count)
	if err != nil {
		logger.Printf("检查用户失败: %v", err)
		return false, nil, fmt.Errorf("检查用户失败: %v", err)
	}

	if count == 0 {
		return false, nil, fmt.Errorf("用户不存在")
	}

	// 生成API密钥（使用UUID作为id）
	keyID := uuid.New().String()
	apiKey := s.generateApiKeyClaudeFormat()
	expiresAt := time.Now().AddDate(0, 0, expiresInDays)
	now := time.Now()

	// 插入API密钥
	insertQuery := `
		INSERT INTO aibasicplatform.aibasicplatform_user_api_keys (id, user_id, api_key, description, created_at, status) 
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err = db.Exec(insertQuery, keyID, userId, apiKey, description, now, 1)
	if err != nil {
		logger.Printf("插入API密钥失败: %v", err)
		return false, nil, fmt.Errorf("创建API密钥失败: %v", err)
	}

	// 更新Redis缓存
	s.updateRedisCacheForUser(userId)

	// 构建返回信息
	keyInfo := map[string]interface{}{
		"id":           keyID,
		"api_key":      apiKey,
		"key":          apiKey,
		"user_id":      userId,
		"description":  description,
		"created_at":   now.Format(time.RFC3339),
		"last_used_at": nil,
		"expires_at":   expiresAt.Format(time.RFC3339),
		"status":       1,
	}

	return true, keyInfo, nil
}

// GetUserApiKeys 获取用户的所有API密钥（保留旧方法兼容性）
// 返回：获取成功，密钥列表，错误消息
func (s *ApiKeyService) GetUserApiKeys(userId string) (bool, []map[string]interface{}, error) {
	return s.ListApiKeys(userId, true)
}

// ListApiKeys 列出用户的API密钥（支持Redis缓存）
// 返回：获取成功，密钥列表，错误消息
func (s *ApiKeyService) ListApiKeys(userId string, includeDeleted bool) (bool, []map[string]interface{}, error) {
	if userId == "" {
		return false, nil, fmt.Errorf("用户ID不能为空")
	}

	var keys []map[string]interface{}

	// 优先从Redis缓存读取
	if s.readWrite != nil {
		redisKey := fmt.Sprintf("api_keys:user:%s", userId)
		keysResult := s.readWrite.GetRedis(redisKey)

		if keysResult != nil && keysResult.IsSuccess() && keysResult.Data != nil {
			if dataStr, ok := keysResult.Data.(string); ok && dataStr != "" {
				if err := json.Unmarshal([]byte(dataStr), &keys); err == nil {
					logger.Printf("从Redis缓存获取到 %d 个API密钥", len(keys))
				} else {
					logger.Printf("解析Redis缓存失败: %v", err)
					keys = nil
				}
			}
		}
	}

	// 如果Redis缓存中没有数据，从数据库读取
	if len(keys) == 0 && s.readWrite != nil {
		query := `
			SELECT id, api_key, description, created_at, last_used_at, status 
			FROM aibasicplatform.aibasicplatform_user_api_keys 
			WHERE user_id = ?
		`
		if !includeDeleted {
			query += " AND status != 2"
		}
		query += " ORDER BY created_at DESC"

		opResult := s.readWrite.QueryDb(query, userId)
		if opResult.IsSuccess() {
			if rows, ok := opResult.Data.([]map[string]interface{}); ok {
				keys = make([]map[string]interface{}, 0, len(rows))
				for _, row := range rows {
					id := fmt.Sprintf("%v", row["id"]) // ensure string
					plainKey, _ := row["api_key"].(string)
					masked := maskApiKey(plainKey)

					// 获取status，确保是数字类型 (0=disabled, 1=active, 2=deleted)
					status := 1
					if s, ok := row["status"].(int64); ok {
						status = int(s)
					}

					key := map[string]interface{}{
						"id":           id,
						"description":  row["description"],
						"api_key":      masked, // 用于列表显示的是脱敏密钥
						"key":          masked, // 兼容前端字段名
						"created_at":   row["created_at"],
						"last_used_at": row["last_used_at"],
						"status":       status,
					}
					keys = append(keys, key)
				}

				// 将数据更新到Redis缓存
				s.updateRedisCacheForUser(userId)

				logger.Printf("从数据库获取到 %d 个API密钥", len(keys))
			}
		} else {
			logger.Printf("从数据库查询API密钥失败: %v", opResult.Error)
		}
	}

	// 统一过滤：仅过滤掉 status=2（deleted），展示 0/1
	if len(keys) > 0 && !includeDeleted {
		filtered := make([]map[string]interface{}, 0, len(keys))
		for _, k := range keys {
			status := 0
			if v, ok := k["status"].(int); ok {
				status = v
			} else if v, ok := k["status"].(int64); ok {
				status = int(v)
			} else if v, ok := k["status"].(float64); ok {
				status = int(v)
			}
			if status != 2 {
				filtered = append(filtered, k)
			}
		}
		keys = filtered
	}

	return true, keys, nil
}

// DisableApiKey 禁用API密钥（保留旧方法兼容性）
// 返回：禁用成功，错误消息
func (s *ApiKeyService) DisableApiKey(userId string, keyId int) (bool, error) {
	return s.UpdateApiKeyStatus(userId, fmt.Sprintf("%d", keyId), 0)
}

// CreateApiKeyWithLimit 创建新的API密钥（带数量限制检查）
// 返回：创建成功，密钥信息，错误消息
func (s *ApiKeyService) CreateApiKeyWithLimit(userId string, keyName string, maxKeys int) (bool, map[string]interface{}, error) {
	if userId == "" {
		return false, nil, fmt.Errorf("用户ID不能为空")
	}
	if keyName == "" {
		return false, nil, fmt.Errorf("API密钥名称不能为空")
	}

	// 前置校验：检查用户已拥有的API密钥数量（启用+禁用，不包含已删除）
	var currentKeyCount int

	if s.readWrite != nil {
		// 优先从Redis缓存检查
		redisKey := fmt.Sprintf("api_keys:user:%s", userId)
		keysResult := s.readWrite.GetRedis(redisKey)

		cacheValid := false
		if keysResult != nil && keysResult.IsSuccess() && keysResult.Data != nil {
			if dataStr, ok := keysResult.Data.(string); ok && dataStr != "" {
				var cachedKeys []map[string]interface{}
				if err := json.Unmarshal([]byte(dataStr), &cachedKeys); err == nil && len(cachedKeys) > 0 {
					cacheValid = true
					// 统计status为0或1的数量
					for _, key := range cachedKeys {
						status := 0
						if v, ok := key["status"].(int); ok {
							status = v
						} else if v, ok := key["status"].(int64); ok {
							status = int(v)
						} else if v, ok := key["status"].(float64); ok {
							status = int(v)
						}
						if status == 0 || status == 1 {
							currentKeyCount++
						}
					}
				}
			}
		}

		// 如果缓存不可用或为空，从数据库查询
		if !cacheValid {
			query := `
				SELECT COUNT(*) as count 
				FROM aibasicplatform.aibasicplatform_user_api_keys 
				WHERE user_id = ? AND (status = 0 OR status = 1)
			`
			opResult := s.readWrite.QueryDb(query, userId)
			if opResult.IsSuccess() {
				if rows, ok := opResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
					if count, ok := rows[0]["count"].(int64); ok {
						currentKeyCount = int(count)
					}
				}
			}
		}
	}

	// 检查是否达到上限
	if currentKeyCount >= maxKeys {
		return false, nil, fmt.Errorf("API密钥数量已达上限（%d个）。请先删除至少1个后再创建。", maxKeys)
	}

	// 生成新的API密钥
	now := time.Now()
	keyID := uuid.New().String()
	apiKey := s.generateApiKeyClaudeFormat()

	// 存储到数据库
	if s.readWrite != nil {
		query := `
			INSERT INTO aibasicplatform.aibasicplatform_user_api_keys (id, user_id, api_key, description, created_at, status) 
			VALUES (?, ?, ?, ?, ?, ?)
		`

		opResult := s.readWrite.ExecuteDb(query, keyID, userId, apiKey, keyName, now, 1)
		if !opResult.IsSuccess() {
			logger.Printf("存储API密钥到数据库失败: %v", opResult.Error)
			return false, nil, fmt.Errorf("创建API密钥失败")
		}

		logger.Printf("API密钥已存储到数据库，ID: %s", keyID)

		// 更新Redis缓存
		s.updateRedisCacheForUser(userId)
	}

	// 构建API密钥对象（用于响应）- 创建时返回完整密钥
	newKey := map[string]interface{}{
		"id":           keyID,
		"description":  keyName,
		"key":          apiKey,           // 创建时返回完整密钥
		"api_key":      apiKey,           // 兼容前端字段名
		"created_at":   now.Format(time.RFC3339),
		"last_used_at": nil,
		"status":       1,                // 使用数字 1=active
	}

	logger.Printf("API密钥创建成功: %s", apiKey)

	return true, newKey, nil
}

// DeleteApiKey 删除API密钥（软删除，status=2）
// 返回：删除成功，错误消息
func (s *ApiKeyService) DeleteApiKey(userId string, keyID string) (bool, error) {
	if userId == "" || keyID == "" {
		return false, fmt.Errorf("用户ID和密钥ID不能为空")
	}

	// 从数据库中软删除（标记为2=deleted）
	if s.readWrite != nil {
		query := `UPDATE aibasicplatform.aibasicplatform_user_api_keys SET status = 2 WHERE id = ? AND user_id = ?`

		opResult := s.readWrite.ExecuteDb(query, keyID, userId)
		if !opResult.IsSuccess() {
			logger.Printf("从数据库删除API密钥失败: %v", opResult.Error)
			return false, fmt.Errorf("删除API密钥失败")
		}

		// 检查是否有行被影响
		if affectedRows, ok := opResult.Data.(int64); ok && affectedRows == 0 {
			logger.Printf("数据库中未找到要删除的API密钥ID: %s", keyID)
			return false, fmt.Errorf("未找到API密钥: %s", keyID)
		}

		logger.Printf("API密钥已从数据库删除，ID: %s", keyID)

		// 更新Redis缓存
		s.updateRedisCacheForUser(userId)
	}

	logger.Printf("API密钥删除成功，ID: %s", keyID)
	return true, nil
}

// UpdateApiKeyStatus 更新API密钥状态（禁用/启用）
// 返回：更新成功，错误消息
func (s *ApiKeyService) UpdateApiKeyStatus(userId string, keyID string, status int) (bool, error) {
	if userId == "" || keyID == "" {
		return false, fmt.Errorf("用户ID和密钥ID不能为空")
	}

	// 验证status值（只允许0或1）
	if status != 0 && status != 1 {
		return false, fmt.Errorf("status值无效（只允许0或1）")
	}

	// 更新数据库中的状态
	if s.readWrite != nil {
		query := `UPDATE aibasicplatform.aibasicplatform_user_api_keys SET status = ? WHERE id = ? AND user_id = ?`

		opResult := s.readWrite.ExecuteDb(query, status, keyID, userId)
		if !opResult.IsSuccess() {
			logger.Printf("更新API密钥状态失败: %v", opResult.Error)
			return false, fmt.Errorf("更新API密钥状态失败")
		}

		// 检查是否有行被更新
		if affectedRows, ok := opResult.Data.(int64); ok && affectedRows == 0 {
			logger.Printf("数据库中未找到要更新的API密钥ID: %s", keyID)
			return false, fmt.Errorf("未找到API密钥: %s", keyID)
		}

		statusText := "禁用"
		if status == 1 {
			statusText = "启用"
		}
		logger.Printf("API密钥状态已从数据库更新，ID: %s, status: %d (%s)", keyID, status, statusText)

		// 更新Redis缓存
		s.updateRedisCacheForUser(userId)
	}

	return true, nil
}

// 生成随机API密钥
func (s *ApiKeyService) generateApiKey(length int) string {
	if length <= 0 {
		length = 48 // 默认长度
	}

	// 定义字符集
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	
	// 创建长度为length的字节切片
	bytes := make([]byte, length)
	
	// 用随机字节填充切片
	_, err := rand.Read(bytes)
	if err != nil {
		logger.Printf("生成随机字节失败: %v", err)
		return ""
	}
	
	// 将随机字节映射到字符集
	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}
	
	return string(bytes)
}

// 获取数据库连接
func (s *ApiKeyService) getDbConnection() (*sql.DB, error) {
	// 构建DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
		s.dbConfig["user"],
		s.dbConfig["password"],
		s.dbConfig["host"],
		s.dbConfig["port"],
		s.dbConfig["database"])

	// 打开数据库连接
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// 设置连接参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// 测试连接
	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateApiKeyClaudeFormat 生成类似Claude格式的API密钥: sk-potagi-api-xxxxx
// 总长度约104个字符（前缀14字符 + 随机部分90字符），与Claude 3的API密钥长度相近
func (s *ApiKeyService) generateApiKeyClaudeFormat() string {
	keyPrefix := "sk-potagi-api-"
	// 生成90个字符的随机字符串（与Claude 3的API密钥长度相近）
	keyRandom := s.generateRandomString(90)
	return fmt.Sprintf("%s%s", keyPrefix, keyRandom)
}

// generateRandomString 生成指定长度的随机字符串（字母和数字）
func (s *ApiKeyService) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	
	// 用随机字节填充
	_, err := rand.Read(bytes)
	if err != nil {
		logger.Printf("生成随机字节失败: %v，使用UUID作为备用", err)
		// 如果随机生成失败，使用多个UUID拼接作为备用方案
		fallback := ""
		for len(fallback) < length {
			uuidStr := removeHyphens(uuid.New().String())
			fallback += uuidStr
		}
		return fallback[:length]
	}
	
	// 将随机字节映射到字符集
	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}
	
	return string(bytes)
}

// maskApiKey 对API密钥进行中间脱敏显示
func maskApiKey(s string) string {
	if s == "" {
		return s
	}
	// 保留前6位与后4位，其余用*
	if len(s) <= 10 {
		return s
	}
	prefix := s[:6]
	suffix := s[len(s)-4:]
	return prefix + strings.Repeat("*", len(s)-10) + suffix
}

// removeHyphens 从字符串中移除连字符
func removeHyphens(s string) string {
	result := ""
	for _, c := range s {
		if c != '-' {
			result += string(c)
		}
	}
	return result
}

// updateRedisCacheForUser 更新用户的Redis缓存
func (s *ApiKeyService) updateRedisCacheForUser(userId string) {
	if s.readWrite == nil {
		return
	}

	// 从数据库重新获取最新数据
	query := `
		SELECT id, api_key, description, created_at, last_used_at, status 
		FROM aibasicplatform.aibasicplatform_user_api_keys 
		WHERE user_id = ? AND status != 2
		ORDER BY created_at DESC
	`

	opResult := s.readWrite.QueryDb(query, userId)
	if !opResult.IsSuccess() {
		logger.Printf("更新Redis缓存时查询数据库失败: %v", opResult.Error)
		return
	}

	var keys []map[string]interface{}
	if rows, ok := opResult.Data.([]map[string]interface{}); ok {
		keys = make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			id := fmt.Sprintf("%v", row["id"]) // ensure string
			plainKey, _ := row["api_key"].(string)
			masked := maskApiKey(plainKey)

			// 获取status，确保是数字类型 (0=disabled, 1=active, 2=deleted)
			status := 1
			if s, ok := row["status"].(int64); ok {
				status = int(s)
			}

			key := map[string]interface{}{
				"id":           id,
				"description":  row["description"],
				"api_key":      masked, // 用于列表显示的是脱敏密钥
				"key":          masked, // 兼容前端字段名
				"created_at":   row["created_at"],
				"last_used_at": row["last_used_at"],
				"status":       status,
			}
			keys = append(keys, key)
		}
	}

	// 将数据更新到Redis缓存
	redisKey := fmt.Sprintf("api_keys:user:%s", userId)
	keysJson, err := json.Marshal(keys)
	if err == nil {
		s.readWrite.SetRedis(redisKey, string(keysJson), 0) // 永不过期
		logger.Printf("API密钥列表已更新到Redis缓存: %s", redisKey)
	} else {
		logger.Printf("更新Redis缓存失败: %v", err)
	}
} 