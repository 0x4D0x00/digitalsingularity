package tokenmanage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"

	"github.com/google/uuid"
	"gopkg.in/ini.v1"
)

// ReadWriteService 接口定义数据读写操作
type ReadWriteService interface {
	GetRedis(key string) string
	SetRedis(key string, value string, expire int) error
	DeleteRedis(key string) error
}

// CommonAuthTokenService 认证令牌管理服务，负责自定义认证令牌的生成、存储、验证和吊销
type CommonAuthTokenService struct {
	readWrite ReadWriteService
	secretKey string
}

// NewCommonAuthTokenService 创建一个新的CommonAuthTokenService实例
func NewCommonAuthTokenService(readWrite ...ReadWriteService) *CommonAuthTokenService {
	var rw ReadWriteService
	if len(readWrite) > 0 {
		rw = readWrite[0]
	}
	if rw == nil {
		rw = newDefaultReadWriteService()
	}

	service := &CommonAuthTokenService{
		readWrite: rw,
	}

	// 从配置文件读取密钥
	configPath := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(os.Args[0]))),
		"config", "backendserviceconfig.ini")

	cfg, err := ini.Load(configPath)
	if err != nil {
		// 在生产环境中应确保配置文件中有密钥
		service.secretKey = "digital_singularity_2025_secure_key"
		fmt.Println("警告: 未能加载配置文件，使用默认密钥。这在生产环境中不安全。")
		return service
	}

	// 使用配置文件中的密钥，如果没有则使用默认值
	section := cfg.Section("TOKEN")
	if section.HasKey("secretKey") {
		service.secretKey = section.Key("secretKey").String()
	} else {
		service.secretKey = "digital_singularity_2025_secure_key"
		fmt.Println("警告: 未在配置文件中找到TOKEN密钥，使用默认密钥。这在生产环境中不安全。")
	}

	return service
}

// defaultReadWriteService 使用 CommonReadWriteService 适配 tokenmanage.ReadWriteService
type defaultReadWriteService struct {
	rw *datahandle.CommonReadWriteService
}

func (d *defaultReadWriteService) GetRedis(key string) string {
	if d == nil || d.rw == nil {
		return ""
	}
	result := d.rw.GetRedis(key)
	if !result.IsSuccess() {
		return ""
	}
	if val, ok := result.Data.(string); ok {
		return val
	}
	return ""
}

func (d *defaultReadWriteService) SetRedis(key string, value string, expire int) error {
	if d == nil || d.rw == nil {
		return fmt.Errorf("默认数据服务未初始化")
	}
	result := d.rw.SetRedis(key, value, time.Duration(expire)*time.Second)
	if !result.IsSuccess() {
		return result.Error
	}
	return nil
}

func (d *defaultReadWriteService) DeleteRedis(key string) error {
	if d == nil || d.rw == nil {
		return fmt.Errorf("默认数据服务未初始化")
	}
	result := d.rw.DeleteRedis(key)
	if !result.IsSuccess() {
		return result.Error
	}
	return nil
}

type noopReadWriteService struct{}

func (noopReadWriteService) GetRedis(string) string {
	return ""
}

func (noopReadWriteService) SetRedis(string, string, int) error {
	return fmt.Errorf("默认数据服务未初始化")
}

func (noopReadWriteService) DeleteRedis(string) error {
	return fmt.Errorf("默认数据服务未初始化")
}

func newDefaultReadWriteService() ReadWriteService {
	rw, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		fmt.Printf("[AuthTokenManage] 初始化默认数据服务失败: %v\n", err)
		return noopReadWriteService{}
	}
	return &defaultReadWriteService{rw: rw}
}

// GenerateAuthToken 生成自定义认证令牌
func (s *CommonAuthTokenService) GenerateAuthToken(user map[string]interface{}) (string, error) {
	userId := user["user_id"].(string)
	jti := uuid.New().String() // 令牌唯一标识符
	timestamp := time.Now().UTC().Unix()
	expiry := time.Now().UTC().Add(7 * 24 * time.Hour).Unix() // 7天后过期

	// 创建令牌原始数据
	authTokenData := fmt.Sprintf("%s:%s:%d:%d", userId, jti, timestamp, expiry)

	// 创建签名 (HMAC-SHA256)
	h := hmac.New(sha256.New, []byte(s.secretKey))
	h.Write([]byte(authTokenData))
	signature := hex.EncodeToString(h.Sum(nil))

	// 组合令牌：数据 + 签名
	authToken := fmt.Sprintf("%s:%s", authTokenData, signature)
	encodedAuthToken := base64.URLEncoding.EncodeToString([]byte(authToken))

	// 将令牌信息保存到Redis，支持令牌验证和吊销
	err := s.saveAuthTokenToRedis(userId, jti, encodedAuthToken, 604800) // 7天过期 (7*24*3600)
	if err != nil {
		return "", err
	}

	return encodedAuthToken, nil
}

// saveAuthTokenToRedis 将认证令牌保存到Redis中，用于验证和吊销管理
func (s *CommonAuthTokenService) saveAuthTokenToRedis(userID, jti, authToken string, expireSeconds int) error {
	// 令牌信息，包含状态、创建时间等
	authTokenData := map[string]interface{}{
		"authToken":  authToken,
		"status":     "active",
		"userId":     userID,
		"createdAt":  time.Now().Format("2006-01-02 15:04:05"),
		"deviceInfo": "未知设备", // 可扩展为记录设备信息
		"ip":         "未知IP", // 可扩展为记录登录IP
	}

	jsonData, err := json.Marshal(authTokenData)
	if err != nil {
		fmt.Printf("保存认证令牌到Redis错误: %s\n", err)
		return err
	}

	// 通过令牌ID存储令牌
	authTokenKey := fmt.Sprintf("authToken:%s", jti)
	err = s.readWrite.SetRedis(authTokenKey, string(jsonData), expireSeconds)
	if err != nil {
		fmt.Printf("保存认证令牌到Redis错误: %s\n", err)
		return err
	}

	// 用户的活跃令牌列表
	userAuthTokensKey := fmt.Sprintf("user:authTokens:%s", userID)
	userAuthTokens := s.readWrite.GetRedis(userAuthTokensKey)

	if userAuthTokens != "" {
		// 更新用户的令牌列表
		var authTokensList []string
		err = json.Unmarshal([]byte(userAuthTokens), &authTokensList)
		if err != nil {
			// 解析错误，创建新列表
			newList := []string{jti}
			jsonList, _ := json.Marshal(newList)
			s.readWrite.SetRedis(userAuthTokensKey, string(jsonList), expireSeconds)
		} else {
			// 限制每个用户的最大活跃令牌数量为5
			if len(authTokensList) >= 5 {
				// 移除最旧的令牌
				oldestJti := authTokensList[len(authTokensList)-1]
				authTokensList = authTokensList[:len(authTokensList)-1]
				// 从Redis中删除最旧的令牌
				s.readWrite.DeleteRedis(fmt.Sprintf("authToken:%s", oldestJti))
			}

			// 将新令牌ID添加到列表开头
			authTokensList = append([]string{jti}, authTokensList...)
			jsonList, _ := json.Marshal(authTokensList)
			s.readWrite.SetRedis(userAuthTokensKey, string(jsonList), expireSeconds)
		}
	} else {
		// 首次创建列表
		newList := []string{jti}
		jsonList, _ := json.Marshal(newList)
		s.readWrite.SetRedis(userAuthTokensKey, string(jsonList), expireSeconds)
	}

	return nil
}

// VerifyAuthToken 验证认证令牌的有效性，并自动刷新即将过期的令牌
func (s *CommonAuthTokenService) VerifyAuthToken(authToken string) (bool, interface{}) {
	// 解码令牌
	authTokenBytes, err := base64.URLEncoding.DecodeString(authToken)
	if err != nil {
		fmt.Printf("解码认证令牌错误: %s\n", err)
		return false, "无效的认证令牌格式"
	}

	authTokenStr := string(authTokenBytes)
	authTokenParts := split(authTokenStr, ":")

	if len(authTokenParts) != 5 { // 应有userId, jti, timestamp, expiry, signature
		return false, "认证令牌格式无效"
	}

	userID, jti, timestampStr, expiryStr, signature := authTokenParts[0], authTokenParts[1], authTokenParts[2], authTokenParts[3], authTokenParts[4]

	// 转换时间戳
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return false, "认证令牌时间戳无效"
	}

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false, "认证令牌过期时间无效"
	}

	// 重新计算签名以验证
	authTokenData := fmt.Sprintf("%s:%s:%s:%s", userID, jti, timestampStr, expiryStr)
	h := hmac.New(sha256.New, []byte(s.secretKey))
	h.Write([]byte(authTokenData))
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// 验证签名
	if signature != expectedSignature {
		return false, "认证令牌签名无效"
	}

	// 检查Redis中的令牌状态（优先检查Redis，因为这里存储了最新的状态）
	authTokenKey := fmt.Sprintf("authToken:%s", jti)
	authTokenDataStr := s.readWrite.GetRedis(authTokenKey)

	if authTokenDataStr == "" {
		// Redis中不存在，再检查authToken本身的过期时间
		currentTime := time.Now().UTC().Unix()
		if currentTime > expiry {
			return false, "认证令牌已过期"
		}
		return false, "认证令牌不存在或已过期"
	}

	var authTokenRedisData map[string]interface{}
	err = json.Unmarshal([]byte(authTokenDataStr), &authTokenRedisData)
	if err != nil {
		return false, "认证令牌数据无效"
	}

	// 检查令牌状态
	status, ok := authTokenRedisData["status"].(string)
	if !ok || status != "active" {
		return false, "认证令牌已被吊销或冻结"
	}

	// 检查当前时间
	currentTime := time.Now().UTC().Unix()

	// 自动刷新机制：检查authToken是否即将过期（剩余时间少于1天）
	remainingTime := expiry - currentTime
	refreshThreshold := int64(24 * 3600) // 1天 = 86400秒

	// 如果authToken本身已过期，但Redis中仍然存在且状态为active，说明是活跃用户，允许通过并刷新
	authTokenExpired := currentTime > expiry
	shouldRefresh := remainingTime < refreshThreshold && remainingTime > 0

	if shouldRefresh || authTokenExpired {
		// AuthToken即将过期或已过期（但Redis中仍然有效），自动刷新（延长Redis中的过期时间）
		// 延长7天
		newExpireSeconds := 604800
		err = s.readWrite.SetRedis(authTokenKey, authTokenDataStr, newExpireSeconds)
		if err != nil {
			fmt.Printf("自动刷新认证令牌过期时间失败: %s\n", err)
			// 如果authToken已过期且刷新失败，则拒绝访问
			if authTokenExpired {
				return false, "认证令牌已过期"
			}
		} else {
			if authTokenExpired {
				fmt.Printf("已自动刷新已过期的认证令牌，用户ID: %s\n", userID)
			} else {
				fmt.Printf("已自动刷新即将过期的认证令牌，用户ID: %s, 剩余时间: %d秒\n", userID, remainingTime)
			}
		}

		// 同时更新用户的authToken列表过期时间
		userAuthTokensKey := fmt.Sprintf("user:authTokens:%s", userID)
		userAuthTokensStr := s.readWrite.GetRedis(userAuthTokensKey)
		if userAuthTokensStr != "" {
			s.readWrite.SetRedis(userAuthTokensKey, userAuthTokensStr, newExpireSeconds)
		}

		// 如果authToken本身已过期，但我们已经在Redis中延长了有效期，允许通过
		if authTokenExpired {
			// 使用当前时间作为新的过期时间基准
			expiry = currentTime + 604800 // 新的7天过期时间
		}
	}

	// 构造返回的payload
	payload := map[string]interface{}{
		"userId": userID,
		"jti":    jti,
		"iat":    timestamp,
		"exp":    expiry,
	}

	return true, payload
}

// RevokeAuthToken 吊销认证令牌
func (s *CommonAuthTokenService) RevokeAuthToken(authToken string) (bool, string) {
	// 解码令牌
	valid, result := s.VerifyAuthToken(authToken)
	if !valid {
		return false, result.(string)
	}

	payload := result.(map[string]interface{})
	jti := payload["jti"].(string)

	// 更新Redis中的令牌状态
	authTokenKey := fmt.Sprintf("authToken:%s", jti)
	authTokenDataStr := s.readWrite.GetRedis(authTokenKey)

	if authTokenDataStr != "" {
		var authTokenData map[string]interface{}
		err := json.Unmarshal([]byte(authTokenDataStr), &authTokenData)
		if err != nil {
			return false, "认证令牌数据无效"
		}

		authTokenData["status"] = "revoked"
		jsonData, _ := json.Marshal(authTokenData)
		s.readWrite.SetRedis(authTokenKey, string(jsonData), 604800) // 保留7天记录
		return true, "认证令牌已成功吊销"
	}

	return false, "认证令牌不存在或已过期"
}

// RefreshAuthToken 刷新认证令牌，生成新令牌并吊销旧令牌
func (s *CommonAuthTokenService) RefreshAuthToken(oldAuthToken string) (bool, interface{}) {
	// 验证旧令牌
	valid, result := s.VerifyAuthToken(oldAuthToken)

	if !valid {
		return false, result
	}

	// 获取用户信息
	payload := result.(map[string]interface{})
	userID := payload["userId"].(string)
	userInfoKey := fmt.Sprintf("user:info:%s", userID)
	userInfoStr := s.readWrite.GetRedis(userInfoKey)

	if userInfoStr == "" {
		return false, "找不到用户信息"
	}

	var user map[string]interface{}
	err := json.Unmarshal([]byte(userInfoStr), &user)
	if err != nil {
		return false, "用户信息无效"
	}

	// 吊销旧令牌
	s.RevokeAuthToken(oldAuthToken)

	// 生成新令牌
	newAuthToken, err := s.GenerateAuthToken(user)
	if err != nil {
		return false, fmt.Sprintf("生成新认证令牌失败: %s", err)
	}

	return true, newAuthToken
}

// split 辅助函数，按照分隔符分割字符串
func split(s, sep string) []string {
	var result []string
	i := 0
	for j := 0; j < len(s); j++ {
		if string(s[j]) == sep {
			result = append(result, s[i:j])
			i = j + 1
		}
	}
	result = append(result, s[i:])
	return result
}
