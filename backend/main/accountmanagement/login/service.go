package login

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	aiplatformdb "digitalsingularity/backend/aibasicplatform/database"
	"digitalsingularity/backend/common/auth/smsverify"
	"digitalsingularity/backend/common/auth/tokenmanage"
	"digitalsingularity/backend/common/security/symmetricencryption/decrypt"
	"digitalsingularity/backend/common/security/symmetricencryption/encrypt"
	"digitalsingularity/backend/common/userinfostorage"
	"digitalsingularity/backend/common/utils/datahandle"
	
	"github.com/google/uuid"
)

// 定义AppDataProvider类型的函数
type AppDataProvider func(user map[string]interface{}) map[string]interface{}

// LoginService 登录服务，处理登录请求和用户认证
type LoginService struct {
	readWrite                  *datahandle.CommonReadWriteService
	authTokenManage            *tokenmanage.CommonAuthTokenService
	userCache                  *userinfostorage.CommonUserCacheService
	smsVerify                  *smsverify.SmsVerifyService
	storageBoxService          interface{}
	aiBasicPlatformDataService *aiplatformdb.AIBasicPlatformDataService
	aiBasicPlatformService     *aiplatformdb.AiBasicPlatformLoginService
	appDataProviders           map[string]AppDataProvider
}

// NewLoginService 创建新的登录服务实例
func NewLoginService(
	readWrite *datahandle.CommonReadWriteService,
	authTokenManage *tokenmanage.CommonAuthTokenService,
	userCache *userinfostorage.CommonUserCacheService,
	smsVerify *smsverify.SmsVerifyService,
	_ interface{}, // 替换blueberriesService参数
	storageBoxService interface{},
	aiBasicPlatformService *aiplatformdb.AiBasicPlatformLoginService,
) *LoginService {
	service := &LoginService{
		readWrite:                  readWrite,
		authTokenManage:            authTokenManage,
		userCache:                  userCache,
		smsVerify:                  smsVerify,
		storageBoxService:          storageBoxService,
		aiBasicPlatformDataService: aiplatformdb.NewAIBasicPlatformDataService(readWrite),
		aiBasicPlatformService:     aiBasicPlatformService,
		appDataProviders:           make(map[string]AppDataProvider),
	}

	// 初始化应用数据提供者
	service.appDataProviders = map[string]AppDataProvider{
		"storagebox":           service.getStorageboxData,
		"aiBasicPlatform":      service.getAIPlatformData,
		"mobile":               service.getStorageboxData,      // 移动端默认使用存储盒数据
		"aiBasicPlatformBasic": service.getAiBasicPlatformData, // 基础平台基础信息
	}

	return service
}

// HandleLoginRequest 是对外暴露的登录入口，保持向后兼容旧调用方
// 其他包应当使用该方法而不是未导出的实现。
func (s *LoginService) HandleLoginRequest(data map[string]interface{}, userPublicKey interface{}) map[string]interface{} {
	return s.handleLoginRequest(data, userPublicKey)
}

// ensureCommunicationSystemUser 确保 communication_system_users 中存在映射记录
// 返回该用户的 user_communication_id
func (s *LoginService) ensureCommunicationSystemUser(userId string) (string, error) {
	if userId == "" {
		return "", fmt.Errorf("用户ID为空")
	}

	// 查询是否已存在映射
	query := `
		SELECT user_communication_id 
		FROM communication_system.communication_system_users 
		WHERE user_id = ?
	`
	opResult := s.readWrite.QueryDb(query, userId)
	if !opResult.IsSuccess() {
		return "", fmt.Errorf("查询通信系统用户映射失败: %v", opResult.Error)
	}

	if rows, ok := opResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
		if v, ok := rows[0]["user_communication_id"].(string); ok && v != "" {
			return v, nil
		}
	}

	// 不存在则创建新的 user_communication_id（UUID v4）
	newCommunicationID := uuid.New().String()
	insertQuery := `
		INSERT INTO communication_system.communication_system_users 
			(user_id, user_communication_id, status, created_at, updated_at)
		VALUES (?, ?, 1, NOW(), NOW())
	`
	insertResult := s.readWrite.ExecuteDb(insertQuery, userId, newCommunicationID)
	if !insertResult.IsSuccess() {
		errStr := strings.ToLower(insertResult.Error.Error())
		// 若唯一约束冲突，可能是并发创建，重查一次
		if !strings.Contains(errStr, "duplicate") {
			return "", fmt.Errorf("创建通信系统用户映射失败: %v", insertResult.Error)
		}

		opResult = s.readWrite.QueryDb(query, userId)
		if !opResult.IsSuccess() {
			return "", fmt.Errorf("重复查询通信系统用户映射失败: %v", opResult.Error)
		}
		if rows, ok := opResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			if v, ok := rows[0]["user_communication_id"].(string); ok && v != "" {
				return v, nil
			}
		}
		return "", fmt.Errorf("通信系统用户映射重复创建失败")
	}

	return newCommunicationID, nil
}

// handleLoginRequest 处理登录请求并返回对应应用数据
func (s *LoginService) handleLoginRequest(data map[string]interface{}, userPublicKey interface{}) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("登录处理异常: %v", r)
		}
	}()

	// 获取请求数据
	phone, _ := data["phone"].(string)

	// 同时支持verifyCode和captcha两种参数名
	var verifyCode string
	if vc, ok := data["verifyCode"].(string); ok {
		verifyCode = vc
	} else if cap, ok := data["captcha"].(string); ok {
		verifyCode = cap
	}

	appType, _ := data["appType"].(string)
	if appType == "" {
		appType = "default"
	}

	// 语言标识已不再使用,翻译数据由其他接口获取

	// 获取调试模式标记，支持布尔值或字符串'true'
	debugMode := false
	if dm, ok := data["debugMode"].(bool); ok {
		debugMode = dm
	} else if dmStr, ok := data["debugMode"].(string); ok {
		debugMode = strings.ToLower(dmStr) == "true"
	} else if isDebug, ok := data["isDebug"].(bool); ok {
		debugMode = isDebug
	} else if isDebugStr, ok := data["isDebug"].(string); ok {
		debugMode = strings.ToLower(isDebugStr) == "true"
	}

	if phone == "" {
		return map[string]interface{}{
			"status":  "fail",
			"message": "手机号不能为空",
		}
	}

	// 正常流程要求验证码，但debug模式下可以跳过验证码检查
	if !debugMode && verifyCode == "" {
		return map[string]interface{}{
			"status":  "fail",
			"message": "验证码不能为空",
		}
	}

	var user map[string]interface{}
	var err error

	// 在debug模式下跳过验证码验证，直接获取或创建用户
	if debugMode {
		log.Printf("调试模式登录: %s", phone)
		user, err = s.getUserOrCreateWithoutVerify(phone)
		if err != nil || user == nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "调试模式下用户获取失败",
			}
		}
	} else {
		// 正常流程：验证用户凭据
		user, err = s.authenticateUser(phone, verifyCode)
		if err != nil || user == nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "手机号或验证码错误",
			}
		}
	}

	// 生成认证令牌
	authToken, err := s.authTokenManage.GenerateAuthToken(user)
	if err != nil {
		return map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("生成认证令牌失败: %v", err),
		}
	}

	// ===== 第一步：统一构建基础用户信息 =====

	// 获取 AI 基础平台的资产数据（token 等）
	var aiPlatformUsageData map[string]interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("获取AI基础平台资产数据错误: %v", r)
			}
		}()
		if s.aiBasicPlatformDataService != nil {
			aiPlatformUsageData = s.aiBasicPlatformDataService.GetUserAIBasicPlatformData(user)
		}
	}()

	if aiPlatformUsageData == nil {
		aiPlatformUsageData = make(map[string]interface{})
	}

	// 安全地解密手机号
	decryptedPhone := ""
	if phoneVal, ok := user["phone"].(string); ok && phoneVal != "" {
		// 尝试解密，如果失败则使用原始值
		decryptedPhoneVal, err := decrypt.SymmetricDecryptService(phoneVal, "", "")
		if err != nil {
			// 解密失败可能是因为数据库中存储的是明文，直接使用数据库中的值
			decryptedPhone = phoneVal
		} else {
			decryptedPhone = decryptedPhoneVal
		}
	} else {
		// 如果数据库中没有phone字段，使用登录时提供的手机号
		decryptedPhone = phone
	}

	// 构建仅包含前端需要的字段的用户信息
	userInfo := map[string]interface{}{
		// 前端真正需要的字段
		"user_name":           user["user_name"], // 用户名（11位数字）
		"nick_name":           user["nickname"],  // 昵称
		"email":               user["email"],
		"mobile_number":       decryptedPhone,
		"user_account":        decryptedPhone,
		"created_at":          s.formatTime(user["created_at"]),
		"last_login":          s.formatTime(user["last_login_at"]),
		"user_level":          "",
		"profile_picture_url": user["avatar_url"],
		"location_region":     "",
		"user_signature":      user["bio"], // 从用户表中的bio字段获取个人简介
		"auth_token":          authToken,
		"is_member":           false, // 默认值
		"member_level":        0,     // 默认值
		"expire_time":         nil,   // 默认值
	}

	// AI基础平台的token数据和storagebox数据不再在登录时返回，由其他接口获取
	log.Printf("登录应用类型: %s", appType)

	// 5. 更新用户应用使用信息
	userID, _ := user["user_id"].(string)
	if userID != "" {
		err := s.updateUserAppUsage(userID, appType)
		if err != nil {
			log.Printf("更新用户应用使用信息错误: %v", err)
		}
	}

	// 确保所有数据都可以被JSON序列化
	completeUserInfo := s.ensureSerializable(userInfo)

	// 将完整的用户信息保存到Redis缓存中（包含所有字段，可能包含敏感信息）
	if userID != "" {
		// 使用redisWrite代替setRedis
		redisKey := fmt.Sprintf("user:complete_data:%s", userID)
		jsonData, _ := json.Marshal(completeUserInfo)
		opResult := s.readWrite.RedisWrite(redisKey, string(jsonData), 86400) // 24小时过期
		if !opResult.IsSuccess() {
			log.Printf("保存完整用户数据到Redis失败: %v", opResult.Error)
		} else {
			log.Printf("已将用户%s的完整数据保存到Redis", userID)
		}
	}

	// 筛选只返回前端需要的字段
	allowedFields := []string{
		"user_name", "nick_name", "email", "mobile_number", "user_account",
		"created_at", "last_login", "user_level",
		"profile_picture_url", "location_region", "user_signature", "auth_token",
		"is_member", "member_level", "expire_time", "location_privacy_settings",
	}

	filteredUserInfo := make(map[string]interface{})
	for _, field := range allowedFields {
		if val, ok := completeUserInfo[field]; ok {
			filteredUserInfo[field] = val
		}
	}

	// 翻译数据和token数据不再在登录时返回，由其他接口获取
	// 返回单一的user_info对象
	return map[string]interface{}{
		"status":    "success",
		"message":   "登录成功",
		"user_info": filteredUserInfo,
	}
}

// authenticateUser 验证用户凭据，如果用户不存在则自动注册
func (s *LoginService) authenticateUser(phone, verifyCode string) (map[string]interface{}, error) {
	// 尝试从缓存中获取用户信息
	cachedUser := s.userCache.GetUserByPhone(phone)

	userId := ""
	var err error

	if cachedUser != nil {
		userIdVal, ok := cachedUser["user_id"].(string)
		if ok {
			userId = userIdVal
		}
	} else {
		// 缓存中没有，需要从数据库查询
		// 加密手机号进行查询
		encryptedPhone, err := encrypt.SymmetricEncryptService(phone, "", "")
		if err != nil {
			log.Printf("手机号加密错误: %v", err)
			return nil, err
		}

		// 指定common数据库
		userQuery := "SELECT * FROM common.users WHERE phone = ?"
		opResult := s.readWrite.QueryDb(userQuery, encryptedPhone)
		if !opResult.IsSuccess() {
			err = opResult.Error
			errorMsg := err.Error()
			if strings.Contains(errorMsg, "Table") && strings.Contains(errorMsg, "doesn't exist") {
				log.Printf("数据库表不存在错误: %s", errorMsg)
				return nil, fmt.Errorf("数据库配置错误，请联系管理员")
			} else if strings.Contains(strings.ToLower(errorMsg), "connection") {
				return nil, fmt.Errorf("数据库连接失败，请稍后再试")
			}
			return nil, err
		}

		userResult, ok := opResult.Data.([]map[string]interface{})
		if !ok || len(userResult) == 0 {
			// 用户不存在，自动注册新用户
			userId, err = s.registerNewUser(phone, encryptedPhone)
			if err != nil || userId == "" {
				return nil, err // 注册失败
			}
		} else {
			user := userResult[0]
			userId, _ = user["user_id"].(string)
		}
	}

	// 验证短信验证码
	valid, message := s.smsVerify.VerifyCode(phone, verifyCode)
	if !valid {
		return nil, fmt.Errorf("验证码验证失败: %s", message)
	}

	// 从数据库获取完整的用户信息
	// 指定common数据库
	userQuery := "SELECT * FROM common.users WHERE user_id = ?"
	opResult := s.readWrite.QueryDb(userQuery, userId)
	if !opResult.IsSuccess() {
		err = opResult.Error
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "Table") && strings.Contains(errorMsg, "doesn't exist") {
			log.Printf("数据库表不存在错误: %s", errorMsg)
			return nil, fmt.Errorf("数据库配置错误，请联系管理员")
		} else if strings.Contains(strings.ToLower(errorMsg), "connection") {
			return nil, fmt.Errorf("数据库连接失败，请稍后再试")
		}
		return nil, err
	}

	userResult, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(userResult) == 0 {
		return nil, fmt.Errorf("用户不存在")
	}

	user := userResult[0]

	// 解密手机号用于内存和缓存使用
	if phoneVal, ok := user["phone"].(string); ok {
		decryptedPhone, err := decrypt.SymmetricDecryptService(phoneVal, "", "")
		if err == nil {
			user["phone"] = decryptedPhone
		}
		// 如果解密失败，保留原始值（可能是明文）
	}

	// 查询用户偏好设置 - 私人助理功能
	preferenceQuery := "SELECT preference_value FROM common.user_preferences WHERE user_id = ? AND preference_key = 'personal_assistant_enabled'"
	prefResult := s.readWrite.QueryDb(preferenceQuery, userId)
	if prefResult.IsSuccess() {
		if prefData, ok := prefResult.Data.([]map[string]interface{}); ok && len(prefData) > 0 {
			if prefValue, ok := prefData[0]["preference_value"].(string); ok {
				user["personal_assistant_enabled"] = prefValue == "1"
			}
		}
	}
	// 如果查询失败或没有记录，默认为false
	if _, exists := user["personal_assistant_enabled"]; !exists {
		user["personal_assistant_enabled"] = false
	}

	// 更新用户最后登录时间
	loginUpdate := "UPDATE common.users SET last_login_at = NOW() WHERE user_id = ?"
	opResult = s.readWrite.ExecuteDb(loginUpdate, userId)
	if !opResult.IsSuccess() {
		log.Printf("更新用户最后登录时间失败: %v", opResult.Error)
	}

	return user, nil
}

// getUserOrCreateWithoutVerify 调试模式下，直接获取或创建用户，不进行验证码验证
func (s *LoginService) getUserOrCreateWithoutVerify(phone string) (map[string]interface{}, error) {
	// 尝试从缓存中获取用户信息
	cachedUser := s.userCache.GetUserByPhone(phone)

	userId := ""
	var err error

	if cachedUser != nil {
		userIdVal, ok := cachedUser["user_id"].(string)
		if ok {
			userId = userIdVal
		}
	} else {
		// 缓存中没有，需要从数据库查询
		encryptedPhone, err := encrypt.SymmetricEncryptService(phone, "", "")
		if err != nil {
			log.Printf("调试模式下手机号加密错误: %v", err)
			return nil, err
		}

		// 指定common数据库
		userQuery := "SELECT * FROM common.users WHERE phone = ?"
		opResult := s.readWrite.QueryDb(userQuery, encryptedPhone)
		if !opResult.IsSuccess() {
			err = opResult.Error
			errorMsg := err.Error()
			if strings.Contains(errorMsg, "Table") && strings.Contains(errorMsg, "doesn't exist") {
				log.Printf("数据库表不存在错误: %s", errorMsg)
				return nil, fmt.Errorf("数据库配置错误，请联系管理员")
			} else if strings.Contains(strings.ToLower(errorMsg), "connection") {
				return nil, fmt.Errorf("数据库连接失败，请稍后再试")
			}
			return nil, err
		}

		userResult, ok := opResult.Data.([]map[string]interface{})
		if !ok || len(userResult) == 0 {
			// 用户不存在，自动注册新用户
			userId, err = s.registerNewUser(phone, encryptedPhone)
			if err != nil || userId == "" {
				return nil, err // 注册失败
			}
		} else {
			user := userResult[0]
			userId, _ = user["user_id"].(string)
		}
	}

	// 直接从数据库获取完整的用户信息
	// 指定common数据库
	userQuery := "SELECT * FROM common.users WHERE user_id = ?"
	opResult := s.readWrite.QueryDb(userQuery, userId)
	if !opResult.IsSuccess() {
		err = opResult.Error
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "Table") && strings.Contains(errorMsg, "doesn't exist") {
			log.Printf("数据库表不存在错误: %s", errorMsg)
			return nil, fmt.Errorf("数据库配置错误，请联系管理员")
		} else if strings.Contains(strings.ToLower(errorMsg), "connection") {
			return nil, fmt.Errorf("数据库连接失败，请稍后再试")
		}
		return nil, err
	}

	userResult, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(userResult) == 0 {
		return nil, fmt.Errorf("调试模式下用户不存在")
	}

	user := userResult[0]

	// 解密手机号用于内存和缓存使用
	if phoneVal, ok := user["phone"].(string); ok {
		decryptedPhone, err := decrypt.SymmetricDecryptService(phoneVal, "", "")
		if err == nil {
			user["phone"] = decryptedPhone
		}
		// 如果解密失败，保留原始值（可能是明文）
	}

	// 查询用户偏好设置 - 私人助理功能
	preferenceQuery := "SELECT preference_value FROM common.user_preferences WHERE user_id = ? AND preference_key = 'personal_assistant_enabled'"
	prefResult := s.readWrite.QueryDb(preferenceQuery, userId)
	if prefResult.IsSuccess() {
		if prefData, ok := prefResult.Data.([]map[string]interface{}); ok && len(prefData) > 0 {
			if prefValue, ok := prefData[0]["preference_value"].(string); ok {
				user["personal_assistant_enabled"] = prefValue == "1"
			}
		}
	}
	// 如果查询失败或没有记录，默认为false
	if _, exists := user["personal_assistant_enabled"]; !exists {
		user["personal_assistant_enabled"] = false
	}

	// 更新用户最后登录时间
	loginUpdate := "UPDATE common.users SET last_login_at = NOW() WHERE user_id = ?"
	opResult = s.readWrite.ExecuteDb(loginUpdate, userId)
	if !opResult.IsSuccess() {
		log.Printf("更新用户最后登录时间失败: %v", opResult.Error)
	}

	return user, nil
}

// registerNewUser 注册新用户并返回用户ID
func (s *LoginService) registerNewUser(phone, encryptedPhone string) (string, error) {
	// 生成用户ID - 11位数字+字母，符合规则：包含至少1个数字、1个小写字母和1个大写字母
	userId := s.generateUserId()

	// 生成用户名 - 11位纯数字，唯一
	userName := s.generateUserName()

	// 生成默认昵称
	var nickname string
	if len(phone) >= 4 {
		nickname = fmt.Sprintf("用户%s", phone[len(phone)-4:]) // 使用手机号后4位
	} else {
		nickname = fmt.Sprintf("用户%s", phone)
	}

	// 插入新用户记录 - 指定common数据库
	insertQuery := `
		INSERT INTO common.users 
		(user_id, user_name, phone, nickname, status, created_at, last_login_at) 
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())
	`
	opResult := s.readWrite.ExecuteDb(insertQuery, userId, userName, encryptedPhone, nickname, 1) // status改为1，表示正常状态
	if !opResult.IsSuccess() {
		err := opResult.Error
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "Table") && strings.Contains(errorMsg, "doesn't exist") {
			return "", fmt.Errorf("数据库配置错误，请联系管理员")
		} else if strings.Contains(errorMsg, "Data too long") {
			return "", fmt.Errorf("用户ID格式错误，系统配置问题")
		} else if strings.Contains(errorMsg, "Duplicate entry") && strings.Contains(errorMsg, "user_name") {
			// 用户名冲突，重试
			return s.registerNewUser(phone, encryptedPhone)
		}
		return "", err
	}

	// 插入默认隐私设置 - 允许通过所有方式搜索
	privacyQuery := `
		INSERT INTO common.user_privacy_settings 
		(user_id, searchable_by_username, searchable_by_phone, searchable_by_email, searchable_by_nickname, created_at, updated_at) 
		VALUES (?, 1, 1, 1, 1, NOW(), NOW())
	`
	privacyResult := s.readWrite.ExecuteDb(privacyQuery, userId)
	if !privacyResult.IsSuccess() {
		log.Printf("创建用户隐私设置失败: %v", privacyResult.Error)
		// 不影响注册流程，只记录日志
	}

	// 插入默认偏好设置 - 私人助理功能默认关闭
	preferenceQuery := `
		INSERT INTO common.user_preferences 
		(user_id, preference_key, preference_value, created_at, updated_at) 
		VALUES (?, 'personal_assistant_enabled', '0', NOW(), NOW())
	`
	preferenceResult := s.readWrite.ExecuteDb(preferenceQuery, userId)
	if !preferenceResult.IsSuccess() {
		log.Printf("创建用户偏好设置失败: %v", preferenceResult.Error)
		// 不影响注册流程，只记录日志
	}

	// 同步创建通信系统用户映射
	if _, err := s.ensureCommunicationSystemUser(userId); err != nil {
		log.Printf("创建通信系统用户映射失败: %v", err)
		// 不终止注册流程，仅记录日志；登录时还会再兜底一次
	}

	log.Printf("自动注册新用户: %s, 用户名: %s, 手机号: %s", userId, userName, phone[len(phone)-4:])
	return userId, nil
}

// generateUserId 生成用户ID - 11位数字+字母，符合规则：包含至少1个数字、1个小写字母和1个大写字母
func (s *LoginService) generateUserId() string {
	// 确保至少有一个数字、一个小写字母、一个大写字母
	mustHave := []rune{
		rune(rand.Intn(10) + '0'), // 一个数字
		rune(rand.Intn(26) + 'a'), // 一个小写字母
		rune(rand.Intn(26) + 'A'), // 一个大写字母
	}

	// 生成剩余8个随机字符
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	remainingChars := make([]rune, 8)
	for i := 0; i < 8; i++ {
		remainingChars[i] = rune(chars[rand.Intn(len(chars))])
	}

	// 合并所有字符并随机排序
	allChars := append(mustHave, remainingChars...)
	rand.Shuffle(len(allChars), func(i, j int) { allChars[i], allChars[j] = allChars[j], allChars[i] })

	// 拼接成11位用户ID
	return string(allChars)
}

// generateUserName 生成用户名 - 11位纯数字，唯一
func (s *LoginService) generateUserName() string {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	userName := make([]byte, 11)
	for i := 0; i < len(userName); i++ {
		userName[i] = chars[rand.Intn(len(chars))]
	}
	return string(userName)
}

// getStorageboxData 获取存储盒应用相关数据，调用专门的服务
func (s *LoginService) getStorageboxData(user map[string]interface{}) map[string]interface{} {
	if s.storageBoxService == nil {
		return map[string]interface{}{
			"error": "storagebox 模块已移除或不可用",
		}
	}
	// 尝试断言为具备 GetUserStorageData 方法的接口
	if svc, ok := s.storageBoxService.(interface {
		GetUserStorageData(map[string]interface{}) map[string]interface{}
	}); ok {
		return svc.GetUserStorageData(user)
	}
	return map[string]interface{}{
		"error": "storagebox 服务不可用",
	}
}

// getAIPlatformData 获取AI基础平台应用相关数据
func (s *LoginService) getAIPlatformData(user map[string]interface{}) map[string]interface{} {
	if s.aiBasicPlatformDataService == nil {
		return map[string]interface{}{}
	}
	return s.aiBasicPlatformDataService.GetUserAIBasicPlatformData(user)
}

// getAiBasicPlatformData 获取AI基础平台应用数据，调用专门的服务
func (s *LoginService) getAiBasicPlatformData(user map[string]interface{}) map[string]interface{} {
	return s.aiBasicPlatformService.GetUserAiBasicPlatformData(user)
}

// updateUserAppUsage 更新用户应用使用信息，只在Redis中记录，不访问数据库
func (s *LoginService) updateUserAppUsage(userId string, appType string) error {
	// 更新Redis中的最近使用应用列表
	redisKey := fmt.Sprintf("user:recent_apps:%s", userId)

	// 获取现有的最近应用列表
	opResult := s.readWrite.GetRedis(redisKey)

	// 如果键不存在，创建新的列表
	if !opResult.IsSuccess() {
		// 键不存在是正常情况（首次登录），创建新列表
		jsonData, _ := json.Marshal([]string{appType})
		setResult := s.readWrite.SetRedis(redisKey, string(jsonData), 86400*30) // 30天过期
		if !setResult.IsSuccess() {
			return setResult.Error
		}
		return nil
	}

	recentApps, ok := opResult.Data.(string)
	if ok && recentApps != "" {
		var recentAppsList []string
		err := json.Unmarshal([]byte(recentApps), &recentAppsList)
		if err == nil {
			// 如果当前应用已在列表中，先移除它
			newList := []string{}
			for _, app := range recentAppsList {
				if app != appType {
					newList = append(newList, app)
				}
			}

			// 将当前应用添加到列表开头
			recentAppsList = append([]string{appType}, newList...)

			// 保留最近使用的5个应用
			if len(recentAppsList) > 5 {
				recentAppsList = recentAppsList[:5]
			}

			// 更新Redis
			jsonData, _ := json.Marshal(recentAppsList)
			setResult := s.readWrite.SetRedis(redisKey, string(jsonData), 86400*30) // 30天过期
			if !setResult.IsSuccess() {
				return setResult.Error
			}
			return nil
		} else {
			// 如果解析错误，重新创建列表
			jsonData, _ := json.Marshal([]string{appType})
			setResult := s.readWrite.SetRedis(redisKey, string(jsonData), 86400*30)
			if !setResult.IsSuccess() {
				return setResult.Error
			}
			return nil
		}
	} else {
		// 首次创建列表
		jsonData, _ := json.Marshal([]string{appType})
		setResult := s.readWrite.SetRedis(redisKey, string(jsonData), 86400*30)
		if !setResult.IsSuccess() {
			return setResult.Error
		}
		return nil
	}
}

// formatTime 格式化时间字段
func (s *LoginService) formatTime(timeValue interface{}) string {
	if timeValue == nil {
		return ""
	}

	switch v := timeValue.(type) {
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ensureSerializable 确保数据可以被JSON序列化
func (s *LoginService) ensureSerializable(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range data {
		result[key] = s.convertToSerializable(value)
	}
	return result
}

// convertToSerializable 将不可序列化的对象转换为可序列化的格式
func (s *LoginService) convertToSerializable(obj interface{}) interface{} {
	if obj == nil {
		return nil
	}

	switch v := obj.(type) {
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = s.convertToSerializable(value)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = s.convertToSerializable(item)
		}
		return result
	default:
		return v
	}
}
