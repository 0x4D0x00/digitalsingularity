package nonce

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
)

// 创建logger
var logger = log.New(log.Writer(), "[Nonce] ", log.LstdFlags)

// NonceService Nonce服务，用于生成和验证一次性令牌
type NonceService struct {
	readWrite         *datahandle.CommonReadWriteService
	nonceExpireSeconds int
	redisKeyPrefix     string
	nonceSecretSuffix  string
}

// NewNonceService 创建新的NonceService实例
func NewNonceService() (*NonceService, error) {
	// 创建读写服务
	readWrite, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		logger.Printf("创建读写服务失败: %v", err)
		return nil, err
	}

	// 创建服务实例
	service := &NonceService{
		readWrite:         readWrite,
		nonceExpireSeconds: 300, // nonce有效期为5分钟
		redisKeyPrefix:     "nonce:",
		nonceSecretSuffix:  "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", // nonce安全后缀
	}

	return service, nil
}

// GenerateNonce 生成一个新的nonce
func (s *NonceService) GenerateNonce() string {
	try := func() string {
		// 生成随机nonce
		randomBytes := make([]byte, 16)
		_, err := rand.Read(randomBytes)
		if err != nil {
			logger.Printf("生成随机字节失败: %v", err)
			return ""
		}
		
		// 转换为十六进制字符串
		nonce := hex.EncodeToString(randomBytes)
		
		// 存储到Redis时添加安全后缀，设置过期时间
		nonceWithSuffix := fmt.Sprintf("%s%s", nonce, s.nonceSecretSuffix)
		redisKey := fmt.Sprintf("%s%s", s.redisKeyPrefix, nonceWithSuffix)
		currentTime := strconv.FormatInt(time.Now().Unix(), 10)
		
		// 设置Redis键
		result := s.readWrite.SetRedis(redisKey, currentTime, time.Duration(s.nonceExpireSeconds)*time.Second)
		if !result.IsSuccess() {
			logger.Printf("存储nonce到Redis失败: %v", result.Error)
			return ""
		}
		
		// 返回给前端的nonce不包含后缀
		return nonce
	}
	
	// 尝试生成nonce
	nonce := try()
	if nonce != "" {
		return nonce
	}
	
	// 出错时尝试生成不用于验证的随机字符串作为备份
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// 极端情况下使用时间戳
		return hex.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	
	return hex.EncodeToString(randomBytes)
}

// VerifyNonce 验证nonce的有效性
func (s *NonceService) VerifyNonce(nonce string) (bool, error) {
	if nonce == "" {
		return false, fmt.Errorf("缺少nonce参数")
	}

	// 检查nonce是否已包含后缀
	nonceToCheck := nonce
	if len(nonce) < len(s.nonceSecretSuffix) || nonce[len(nonce)-len(s.nonceSecretSuffix):] != s.nonceSecretSuffix {
		// 未包含后缀，添加后缀
		nonceToCheck = fmt.Sprintf("%s%s", nonce, s.nonceSecretSuffix)
	}

	// 检查Redis中是否存在该nonce
	redisKey := fmt.Sprintf("%s%s", s.redisKeyPrefix, nonceToCheck)
	result := s.readWrite.GetRedis(redisKey)
	
	if !result.IsSuccess() {
		return false, fmt.Errorf("无效的nonce或已过期")
	}
	
	// 获取nonce时间戳
	nonceTimeStr, ok := result.Data.(string)
	if !ok {
		return false, fmt.Errorf("nonce数据类型错误")
	}

	// nonce验证成功后立即删除，确保一次性使用
	deleteResult := s.readWrite.DeleteRedis(redisKey)
	if !deleteResult.IsSuccess() {
		logger.Printf("删除已使用的nonce失败: %v", deleteResult.Error)
	}

	// 检查nonce是否超过有效期
	nonceTime, err := strconv.ParseInt(nonceTimeStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("解析nonce时间戳失败: %v", err)
	}
	
	currentTime := time.Now().Unix()
	if currentTime-nonceTime > int64(s.nonceExpireSeconds) {
		return false, fmt.Errorf("nonce已过期")
	}

	return true, nil
}

// CleanExpiredNonces 清理已过期的nonce（通常不需要手动调用，Redis会自动过期）
func (s *NonceService) CleanExpiredNonces() int {
	// 此方法通常不需要实现，因为Redis会自动删除过期的键
	// 但作为管理功能预留
	return 0
} 