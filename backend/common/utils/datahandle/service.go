package datahandle

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"digitalsingularity/backend/common/configs/settings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-redis/redis/v8"
	"golang.org/x/net/context"
)

// OperationStatus 定义操作状态
type OperationStatus int

const (
	StatusSuccess OperationStatus = iota + 1
	StatusFailure
	StatusTimeout
	StatusConnectionError
	StatusNotFound
)

// OperationResult 操作结果封装类
type OperationResult struct {
	Status OperationStatus
	Data   interface{}
	Error  error
}

// IsSuccess 返回操作是否成功
func (r *OperationResult) IsSuccess() bool {
	return r.Status == StatusSuccess
}

// CommonReadWriteService 数据库和Redis读写封装类
type CommonReadWriteService struct {
	dbConfig    map[string]interface{}
	redisConfig map[string]interface{}
	db          *sql.DB
	redisClient *redis.Client
	mutex       sync.Mutex
	ctx         context.Context
}

// NewCommonReadWriteService 创建一个新的CommonReadWriteService实例
func NewCommonReadWriteService(databaseSection string) (*CommonReadWriteService, error) {
	service := &CommonReadWriteService{
		ctx: context.Background(),
	}

	// 读取配置文件
	if err := service.loadConfig(databaseSection); err != nil {
		return nil, err
	}

	return service, nil
}

// getCommonSettings 获取应用配置实例
func (s *CommonReadWriteService) getCommonSettings() *settings.CommonSettings {
	return settings.NewCommonSettings()
}

// GetDbConfig 获取数据库配置（公开方法）
func (s *CommonReadWriteService) GetDbConfig() map[string]interface{} {
	return s.dbConfig
}

// 加载配置文件
func (s *CommonReadWriteService) loadConfig(databaseSection string) error {
	// 使用应用配置系统
	commonSettings := settings.NewCommonSettings()

	// 设置数据库配置（使用应用配置）
	s.dbConfig = map[string]interface{}{
		"host":     commonSettings.DbHost,
		"port":     commonSettings.DbPort,
		"user":     commonSettings.DbUser,
		"password": commonSettings.DbPassword,
	}

	// 根据原始数据库部分名称确定具体的数据库名
	var databaseName string
	switch databaseSection {
	case "database", "silicoid":
		// 默认使用silicoid数据库
		databaseName = commonSettings.DbNameSilicoid
	case "common":
		databaseName = commonSettings.DbName
	case "communication_system":
		// 通信系统数据库
		databaseName = commonSettings.DbNameCommunication
	case "storagebox":
		databaseName = "storagebox" // Storagebox数据库固定名称
	default:
		// 如果指定的数据库部分不存在，尝试直接读取
		// 最后降级到silicoid数据库
		databaseName = commonSettings.DbNameSilicoid
	}

	s.dbConfig["database"] = databaseName

	// 设置Redis配置
	s.redisConfig = map[string]interface{}{
		"host":     commonSettings.RedisHost,
		"port":     commonSettings.RedisPort,
		"password": commonSettings.RedisPassword,
		"db":       commonSettings.RedisDb,
	}

	return nil
}

// getDbConnection 获取数据库连接
func (s *CommonReadWriteService) getDbConnection() (*sql.DB, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.db == nil {
		// 构建DSN
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
			s.dbConfig["user"],
			s.dbConfig["password"],
			s.dbConfig["host"],
			s.dbConfig["port"],
			s.dbConfig["database"])

		// 创建数据库连接
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, err
		}

		// 设置连接池参数
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)

		// 测试连接
		if err := db.Ping(); err != nil {
			db.Close()
			return nil, err
		}

		s.db = db
	}

	return s.db, nil
}

// GetRedisConnection 获取Redis连接（公开方法）
func (s *CommonReadWriteService) GetRedisConnection() (*redis.Client, error) {
	return s.getRedisConnection()
}

// getRedisConnection 获取Redis连接（内部方法）
func (s *CommonReadWriteService) getRedisConnection() (*redis.Client, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.redisClient == nil {
		// 创建Redis客户端
		client := redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", s.redisConfig["host"], s.redisConfig["port"]),
			Password: s.redisConfig["password"].(string),
			DB:       s.redisConfig["db"].(int),
		})

		// 测试连接
		_, err := client.Ping(s.ctx).Result()
		if err != nil {
			return nil, err
		}

		s.redisClient = client
	}

	return s.redisClient, nil
}

// handleError 错误处理
func (s *CommonReadWriteService) handleError(err error) *OperationResult {
	if err == nil {
		return &OperationResult{Status: StatusSuccess}
	}

	// 静默处理Redis只读副本错误
	if err.Error() == "READONLY You can't write against a read only replica." {
		return &OperationResult{Status: StatusSuccess} // 静默跳过只读副本错误
	}

	log.Printf("操作错误: %v", err)

	// 可以根据错误类型进行更细致的分类
	if err == sql.ErrNoRows {
		return &OperationResult{Status: StatusNotFound, Error: err}
	}

	// 判断是否为连接错误
	if err.Error() == "sql: database is closed" || err == redis.ErrClosed {
		return &OperationResult{Status: StatusConnectionError, Error: err}
	}

	return &OperationResult{Status: StatusFailure, Error: err}
}

// QueryDb 执行数据库查询
func (s *CommonReadWriteService) QueryDb(query string, params ...interface{}) *OperationResult {
	db, err := s.getDbConnection()
	if err != nil {
		return s.handleError(err)
	}

	// 执行查询
	rows, err := db.Query(query, params...)
	if err != nil {
		return s.handleError(err)
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return s.handleError(err)
	}

	// 准备结果集
	var results []map[string]interface{}

	// 遍历结果集
	for rows.Next() {
		// 为每一行创建一个值的切片
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		// 扫描行内容到值切片
		if err := rows.Scan(valuePtrs...); err != nil {
			return s.handleError(err)
		}

		// 创建行映射
		row := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			row[col] = v
		}

		results = append(results, row)
	}

	// 检查迭代错误
	if err := rows.Err(); err != nil {
		return s.handleError(err)
	}

	return &OperationResult{Status: StatusSuccess, Data: results}
}

// ExecuteDb 执行数据库更新操作
func (s *CommonReadWriteService) ExecuteDb(query string, params ...interface{}) *OperationResult {
	db, err := s.getDbConnection()
	if err != nil {
		return s.handleError(err)
	}

	// 执行更新
	result, err := db.Exec(query, params...)
	if err != nil {
		return s.handleError(err)
	}

	// 获取受影响的行数
	affectedRows, err := result.RowsAffected()
	if err != nil {
		return s.handleError(err)
	}

	return &OperationResult{Status: StatusSuccess, Data: affectedRows}
}

// ExecuteDbAsync 异步执行数据库更新操作
func (s *CommonReadWriteService) ExecuteDbAsync(query string, params ...interface{}) chan *OperationResult {
	resultChan := make(chan *OperationResult, 1)

	go func() {
		result := s.ExecuteDb(query, params...)
		resultChan <- result
	}()

	return resultChan
}

// GetRedis 从Redis获取值
func (s *CommonReadWriteService) GetRedis(key string) *OperationResult {
	client, err := s.getRedisConnection()
	if err != nil {
		return s.handleError(err)
	}

	// 获取值
	val, err := client.Get(s.ctx, key).Result()
	if err == redis.Nil {
		return &OperationResult{Status: StatusNotFound, Error: fmt.Errorf("键不存在: %s", key)}
	} else if err != nil {
		return s.handleError(err)
	}

	return &OperationResult{Status: StatusSuccess, Data: val}
}

// RedisRead 从Redis读取数据，返回存储的对象
func (s *CommonReadWriteService) RedisRead(key string) *OperationResult {
	// 先尝试使用常规get方法获取
	result := s.GetRedis(key)
	if result.Status != StatusSuccess {
		return result
	}

	strValue, ok := result.Data.(string)
	if !ok {
		return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("值类型错误")}
	}

	// 尝试将字符串转换为JSON
	var jsonData interface{}
	if err := json.Unmarshal([]byte(strValue), &jsonData); err == nil {
		return &OperationResult{Status: StatusSuccess, Data: jsonData}
	}

	// 如果不是JSON，返回原始字符串
	return &OperationResult{Status: StatusSuccess, Data: strValue}
}

// RedisWrite 将值写入Redis，支持复杂数据结构
func (s *CommonReadWriteService) RedisWrite(key string, value interface{}, expire time.Duration) *OperationResult {
	client, err := s.getRedisConnection()
	if err != nil {
		return s.handleError(err)
	}

	// 如果是复杂数据结构，转换为JSON字符串
	var strValue string
	switch value := value.(type) {
	case string:
		strValue = value
	default:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return s.handleError(err)
		}
		strValue = string(jsonBytes)
	}

	// 设置值
	err = client.Set(s.ctx, key, strValue, expire).Err()
	if err != nil {
		return s.handleError(err)
	}

	return &OperationResult{Status: StatusSuccess}
}

// SetRedis 设置Redis键值
func (s *CommonReadWriteService) SetRedis(key string, value string, expire time.Duration) *OperationResult {
	client, err := s.getRedisConnection()
	if err != nil {
		return s.handleError(err)
	}

	// 设置值
	err = client.Set(s.ctx, key, value, expire).Err()
	if err != nil {
		return s.handleError(err)
	}

	return &OperationResult{Status: StatusSuccess}
}

// DeleteRedis 删除Redis键
func (s *CommonReadWriteService) DeleteRedis(key string) *OperationResult {
	client, err := s.getRedisConnection()
	if err != nil {
		return s.handleError(err)
	}

	// 删除键
	count, err := client.Del(s.ctx, key).Result()
	if err != nil {
		return s.handleError(err)
	}

	return &OperationResult{Status: StatusSuccess, Data: count}
}

// ProcessDbOperation 处理数据库操作
func (s *CommonReadWriteService) ProcessDbOperation(operationType string, args ...interface{}) *OperationResult {
	switch operationType {
	case "query":
		if len(args) < 1 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("查询需要SQL语句参数")}
		}
		query, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("查询参数必须是字符串")}
		}
		return s.QueryDb(query, args[1:]...)
	case "execute":
		if len(args) < 1 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("执行需要SQL语句参数")}
		}
		query, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("执行参数必须是字符串")}
		}
		return s.ExecuteDb(query, args[1:]...)
	default:
		return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("不支持的操作类型: %s", operationType)}
	}
}

// ProcessRedisOperation 处理Redis操作
func (s *CommonReadWriteService) ProcessRedisOperation(operationType string, args ...interface{}) *OperationResult {
	switch operationType {
	case "get":
		if len(args) < 1 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("get需要键参数")}
		}
		key, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("键参数必须是字符串")}
		}
		return s.GetRedis(key)
	case "read":
		if len(args) < 1 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("read需要键参数")}
		}
		key, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("键参数必须是字符串")}
		}
		return s.RedisRead(key)
	case "set":
		if len(args) < 2 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("set需要键和值参数")}
		}
		key, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("键参数必须是字符串")}
		}
		value, ok := args[1].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("值参数必须是字符串")}
		}
		var expire time.Duration
		if len(args) > 2 {
			expireVal, ok := args[2].(int)
			if ok {
				expire = time.Duration(expireVal) * time.Second
			}
		}
		return s.SetRedis(key, value, expire)
	case "write":
		if len(args) < 2 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("write需要键和值参数")}
		}
		key, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("键参数必须是字符串")}
		}
		var expire time.Duration
		if len(args) > 2 {
			expireVal, ok := args[2].(int)
			if ok {
				expire = time.Duration(expireVal) * time.Second
			}
		}
		return s.RedisWrite(key, args[1], expire)
	case "delete":
		if len(args) < 1 {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("delete需要键参数")}
		}
		key, ok := args[0].(string)
		if !ok {
			return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("键参数必须是字符串")}
		}
		return s.DeleteRedis(key)
	default:
		return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("不支持的Redis操作类型: %s", operationType)}
	}
}

// Close 关闭数据库和Redis连接
func (s *CommonReadWriteService) Close() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.db != nil {
		s.db.Close()
		s.db = nil
	}

	if s.redisClient != nil {
		s.redisClient.Close()
		s.redisClient = nil
	}
}

// GetServerPublicKey 获取服务器公钥
func (s *CommonReadWriteService) GetServerPublicKey() *OperationResult {
	log.Println("开始获取服务器公钥")

	// 使用应用配置服务获取公钥文件路径
	commonSettings := s.getCommonSettings()
	publicKeyPath := commonSettings.GetConfigPath("server_public_key.pem")
	
	log.Printf("尝试从路径读取公钥: %s", publicKeyPath)

	// 检查文件是否存在
	if _, err := os.Stat(publicKeyPath); err != nil {
		log.Printf("公钥文件不存在: %v", err)
		return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("无法找到服务器公钥文件: %v", err)}
	}

	// 读取公钥文件
	publicKeyPem, err := os.ReadFile(publicKeyPath)
	if err != nil {
		log.Printf("从 %s 读取公钥失败: %v", publicKeyPath, err)
		return &OperationResult{Status: StatusFailure, Error: fmt.Errorf("读取公钥文件失败: %v", err)}
	}

	// 将PEM内容转为hex
	hexKey := hex.EncodeToString(publicKeyPem)
	log.Printf("服务器公钥获取成功，文件路径: %s，hex长度: %d", publicKeyPath, len(hexKey))
	return &OperationResult{Status: StatusSuccess, Data: hexKey}
}

// RoleInfo 角色信息结构
type RoleInfo struct {
	RoleType string `json:"role_type"` // 角色类型（如：comprehensive_strike）
	RoleName string `json:"role_name"` // 角色名称（如：全面打击）
}

// ClearSystemPromptKeys 清空Redis中所有system_prompt相关的键
// 用于应用启动时清理旧数据，包括 system_prompt:* 和 role:internal:* 键
func (s *CommonReadWriteService) ClearSystemPromptKeys() error {
	client, err := s.getRedisConnection()
	if err != nil {
		return fmt.Errorf("获取Redis连接失败: %v", err)
	}

	// 清空 system_prompt:* 键
	pattern := "system_prompt:*"
	var cursor uint64
	var deletedCount int
	for {
		var keys []string
		var scanErr error
		keys, cursor, scanErr = client.Scan(s.ctx, cursor, pattern, 100).Result()
		if scanErr != nil {
			return fmt.Errorf("扫描Redis键失败: %v", scanErr)
		}

		if len(keys) > 0 {
			deleted, err := client.Del(s.ctx, keys...).Result()
			if err != nil {
				return fmt.Errorf("删除Redis键失败: %v", err)
			}
			deletedCount += int(deleted)
		}

		if cursor == 0 {
			break
		}
	}

	// 清空 role:internal:* 键
	pattern = "role:internal:*"
	cursor = 0
	for {
		var keys []string
		var scanErr error
		keys, cursor, scanErr = client.Scan(s.ctx, cursor, pattern, 100).Result()
		if scanErr != nil {
			return fmt.Errorf("扫描Redis键失败: %v", scanErr)
		}

		if len(keys) > 0 {
			deleted, err := client.Del(s.ctx, keys...).Result()
			if err != nil {
				return fmt.Errorf("删除Redis键失败: %v", err)
			}
			deletedCount += int(deleted)
		}

		if cursor == 0 {
			break
		}
	}

	log.Printf("已清空 %d 个system_prompt相关键", deletedCount)
	return nil
}

// GetAllRolesFromRedis 从Redis获取所有外部可用角色（供前端使用）
// 通过扫描所有 system_prompt:* 的key来获取角色列表
// 注意：所有服务统一使用 role_name 作为 Redis key (system_prompt:{role_name})
// 从 JSON 格式的数据中读取 is_internal 字段，只返回 is_internal=0 的外部角色
// 从 JSON 格式的数据中读取 role_type 字段作为角色类型标识
func (s *CommonReadWriteService) GetAllRolesFromRedis() ([]RoleInfo, error) {
	client, err := s.getRedisConnection()
	if err != nil {
		return nil, fmt.Errorf("获取Redis连接失败: %v", err)
	}

	// 使用SCAN命令遍历所有system_prompt:*的key
	var cursor uint64
	var roles []RoleInfo
	pattern := "system_prompt:*"
	seenRoles := make(map[string]bool) // 用于去重

	for {
		var keys []string
		var scanErr error
		
		keys, cursor, scanErr = client.Scan(s.ctx, cursor, pattern, 100).Result()
		if scanErr != nil {
			return nil, fmt.Errorf("扫描Redis键失败: %v", scanErr)
		}

		// 处理找到的key
		for _, key := range keys {
			// 跳过无效的key
			if len(key) <= len("system_prompt:") {
				log.Printf("[GetAllRolesFromRedis] 跳过无效的key: %s", key)
				continue
			}
			
			// 从Redis读取角色信息
			result := s.RedisRead(key)
			if !result.IsSuccess() {
				log.Printf("[GetAllRolesFromRedis] 从Redis读取key %s 失败: %v", key, result.Error)
				continue
			}
			
			var roleInfo map[string]interface{}
			
			// 处理两种情况：数据可能是string（JSON字符串）或map[string]interface{}（已解析的对象）
			switch data := result.Data.(type) {
			case string:
				// 如果是字符串，需要解析JSON
				if data == "" {
					log.Printf("[GetAllRolesFromRedis] key %s 的数据为空", key)
					continue
				}
				if err := json.Unmarshal([]byte(data), &roleInfo); err != nil {
					log.Printf("[GetAllRolesFromRedis] key %s 的数据不是有效的JSON格式，跳过: %v, 数据前100字符: %s", key, err, getFirstNChars(data, 100))
					continue
				}
			case map[string]interface{}:
				// 如果已经是map类型，直接使用
				roleInfo = data
			default:
				log.Printf("[GetAllRolesFromRedis] key %s 的数据类型不支持，类型: %T", key, result.Data)
				continue
			}
			
			// 提取 role_name 作为角色名称（用于去重和返回）
			roleName := ""
			if name, ok := roleInfo["role_name"].(string); ok && name != "" {
				roleName = name
			} else {
				// 如果JSON中没有role_name字段，从key中提取（因为key格式是 system_prompt:{role_name}）
				keySuffix := key[len("system_prompt:"):]
				roleName = keySuffix
				log.Printf("[GetAllRolesFromRedis] key %s 的JSON中没有role_name字段，使用key后缀: %s", key, keySuffix)
			}
			
			// ⚠️ 关键修复：使用 role_name 去重，而不是 role_type
			// 因为现在所有服务统一使用 role_name 作为 Redis key，同一个 role_name 不应该重复
			if roleName == "" {
				log.Printf("[GetAllRolesFromRedis] key %s 无法确定role_name，跳过", key)
				continue
			}
			
			if seenRoles[roleName] {
				log.Printf("[GetAllRolesFromRedis] 跳过重复的角色: role_name=%s (key: %s)", roleName, key)
				continue
			}
			
			// 从JSON中读取role_type，如果没有就保持为空字符串
			roleType := ""
			if rt, ok := roleInfo["role_type"].(string); ok && rt != "" {
				roleType = rt
			}
			
			// 检查 is_internal 字段，如果没有就默认为0
			// 添加调试日志，打印实际类型和值
			if roleInfo["is_internal"] != nil {
				log.Printf("[GetAllRolesFromRedis] 🔍 DEBUG role_name=%s (key: %s) is_internal 类型: %T, 值: %v", roleName, key, roleInfo["is_internal"], roleInfo["is_internal"])
			}
			isInternal := 0
			if internal, ok := roleInfo["is_internal"].(float64); ok {
				isInternal = int(internal)
			} else if internal, ok := roleInfo["is_internal"].(int); ok {
				isInternal = internal
			} else if internal, ok := roleInfo["is_internal"].(int64); ok {
				isInternal = int(internal)
			} else if internal, ok := roleInfo["is_internal"].(bool); ok {
				if internal {
					isInternal = 1
				}
			} else if internalStr, ok := roleInfo["is_internal"].(string); ok {
				// 处理字符串类型：可能是 "1", "0", "true", "false" 等
				if internalStr == "1" || internalStr == "true" || internalStr == "True" || internalStr == "TRUE" {
					isInternal = 1
				} else if internalStr == "0" || internalStr == "false" || internalStr == "False" || internalStr == "FALSE" {
					isInternal = 0
				} else {
					log.Printf("[GetAllRolesFromRedis] ⚠️ role_name=%s (key: %s) 的 is_internal 字段为字符串但值无法识别: %q，默认视为0", roleName, key, internalStr)
				}
			} else if roleInfo["is_internal"] != nil {
				// 如果字段存在但类型不匹配，记录警告并显示实际类型和值
				log.Printf("[GetAllRolesFromRedis] ⚠️ role_name=%s (key: %s) 的 is_internal 字段类型异常: %T, 值: %v，默认视为0", roleName, key, roleInfo["is_internal"], roleInfo["is_internal"])
			}
			
			// 只返回外部角色（is_internal=0）
			if isInternal != 0 {
				log.Printf("[GetAllRolesFromRedis] 跳过内部角色: role_name=%s (key: %s, is_internal=%d)", roleName, key, isInternal)
				continue
			}
			
			log.Printf("[GetAllRolesFromRedis] ✅ 添加角色: role_type=%s, role_name=%s (key: %s, is_internal=%d)", roleType, roleName, key, isInternal)
			seenRoles[roleName] = true
			roles = append(roles, RoleInfo{
				RoleType: roleType, // 从JSON中读取的角色类型
				RoleName: roleName, // 从JSON中读取的角色名称
			})
		}

		// 如果cursor为0，说明扫描完成
		if cursor == 0 {
			break
		}
	}

	return roles, nil
}

// SystemPromptInfo 系统提示词信息
type SystemPromptInfo struct {
	RoleName     string `json:"role_name"`     // 角色名称
	RoleType     string `json:"role_type"`     // 角色类型
	SystemPrompt string `json:"system_prompt"` // 系统提示词
	IsInternal   int    `json:"is_internal"`   // 是否为内部角色（0=外部，1=内部）
}

// GetAllSystemPromptsFromRedis 从Redis获取所有系统提示词
// 通过扫描所有 system_prompt:* 的key来获取系统提示词列表
// 返回所有角色的系统提示词（包括内部和外部角色）
func (s *CommonReadWriteService) GetAllSystemPromptsFromRedis() ([]SystemPromptInfo, error) {
	client, err := s.getRedisConnection()
	if err != nil {
		return nil, fmt.Errorf("获取Redis连接失败: %v", err)
	}

	ctx := s.ctx

	// 使用SCAN命令遍历所有system_prompt:*的key
	var cursor uint64
	var prompts []SystemPromptInfo
	pattern := "system_prompt:*"

	for {
		var keys []string
		var scanErr error

		keys, cursor, scanErr = client.Scan(ctx, cursor, pattern, 100).Result()
		if scanErr != nil {
			return nil, fmt.Errorf("扫描Redis键失败: %v", scanErr)
		}

		// 处理找到的key
		for _, key := range keys {
			// 跳过无效的key
			if len(key) <= len("system_prompt:") {
				continue
			}
			
			// 从Redis获取角色信息
			result := s.RedisRead(key)
			if !result.IsSuccess() {
				continue
			}
			
			var roleInfo map[string]interface{}
			
			// 处理两种情况：数据可能是string（JSON字符串）或map[string]interface{}（已解析的对象）
			switch data := result.Data.(type) {
			case string:
				// 如果是字符串，需要解析JSON
				if data == "" {
					continue
				}
				if err := json.Unmarshal([]byte(data), &roleInfo); err != nil {
					log.Printf("[GetAllSystemPromptsFromRedis] key %s 的数据不是有效的JSON格式，跳过: %v", key, err)
					continue
				}
			case map[string]interface{}:
				// 如果已经是map类型，直接使用
				roleInfo = data
			default:
				log.Printf("[GetAllSystemPromptsFromRedis] key %s 的数据类型不支持，类型: %T", key, result.Data)
				continue
			}
			
			// 提取 role_name：优先从JSON中读取，如果没有则从key中提取
			roleName := ""
			if name, ok := roleInfo["role_name"].(string); ok && name != "" {
				roleName = name
			} else {
				// 如果JSON中没有role_name字段，从key中提取（因为key格式是 system_prompt:{role_name}）
				keySuffix := key[len("system_prompt:"):]
				roleName = keySuffix
			}
			
			// 提取 role_type：从JSON中读取
			roleType := ""
			if rt, ok := roleInfo["role_type"].(string); ok && rt != "" {
				roleType = rt
			}
			
			// 提取 is_internal 字段
			isInternal := 0
			if internal, ok := roleInfo["is_internal"].(float64); ok {
				isInternal = int(internal)
			} else if internal, ok := roleInfo["is_internal"].(int); ok {
				isInternal = internal
			} else if internal, ok := roleInfo["is_internal"].(int64); ok {
				isInternal = int(internal)
			} else if internal, ok := roleInfo["is_internal"].(bool); ok {
				if internal {
					isInternal = 1
				}
			} else if internalStr, ok := roleInfo["is_internal"].(string); ok {
				if internalStr == "1" || internalStr == "true" || internalStr == "True" || internalStr == "TRUE" {
					isInternal = 1
				}
			}
			
			// 提取 system_prompt 字段（返回所有角色，包括内部和外部）
			systemPrompt, ok := roleInfo["system_prompt"].(string)
			if ok && systemPrompt != "" {
				prompts = append(prompts, SystemPromptInfo{
					RoleName:     roleName,
					RoleType:     roleType,
					SystemPrompt: systemPrompt,
					IsInternal:   isInternal,
				})
			}
		}

		// 如果cursor为0，说明扫描完成
		if cursor == 0 {
			break
		}
	}

	return prompts, nil
}

// getFirstNChars 获取字符串的前N个字符，用于日志输出
func getFirstNChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
} 