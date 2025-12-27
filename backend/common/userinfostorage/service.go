package userinfostorage

import (
	"encoding/json"
	"fmt"
	"time"
)

// ReadWriteService 接口定义数据读写操作
type ReadWriteService interface {
	GetRedis(key string) string
	SetRedis(key string, value string, expire int) error
	DeleteRedis(key string) error
}

// SymmetricEncryptService 对称加密服务接口
type SymmetricEncryptFunc func(plainText string) (string, error)

// SymmetricDecryptService 对称解密服务接口
type SymmetricDecryptFunc func(cipherText string) (string, error)

// CommonUserCacheService 用户信息缓存服务，负责用户信息在Redis中的存储和获取
type CommonUserCacheService struct {
	readWrite          ReadWriteService
	encryptService     SymmetricEncryptFunc
	decryptService     SymmetricDecryptFunc
	cacheExpireSeconds int
}

// NewCommonUserCacheService 创建一个新的CommonUserCacheService实例
func NewCommonUserCacheService(readWrite ReadWriteService, encryptService SymmetricEncryptFunc, decryptService SymmetricDecryptFunc) *CommonUserCacheService {
	return &CommonUserCacheService{
		readWrite:          readWrite,
		encryptService:     encryptService,
		decryptService:     decryptService,
		cacheExpireSeconds: 86400 * 7, // 7天
	}
}

// CacheUserInfo 将用户基本信息缓存到Redis中，方便其他服务使用
func (s *CommonUserCacheService) CacheUserInfo(user map[string]interface{}) (bool, string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("缓存用户信息错误: %v\n", r)
		}
	}()

	userId, ok := user["user_id"].(string)
	if !ok {
		return false, "用户ID不合法"
	}

	phone, _ := user["phone"].(string)

	// 用户基本信息缓存
	userInfo := map[string]interface{}{
		"user_id":    userId,
		"nickname":   user["nickname"],
		"phone":      phone, // 存储原始手机号，仅在内存中使用
		"email":      user["email"],
		"avatar_url": user["avatar_url"],
		"last_login": time.Now().Format("2006-01-02 15:04:05"),
	}

	// 保存用户基本信息
	redisKey := fmt.Sprintf("user:info:%s", userId)
	jsonData, err := json.Marshal(userInfo)
	if err != nil {
		return false, fmt.Sprintf("缓存用户信息错误: %s", err)
	}

	err = s.readWrite.SetRedis(redisKey, string(jsonData), s.cacheExpireSeconds)
	if err != nil {
		return false, fmt.Sprintf("缓存用户信息错误: %s", err)
	}

	// 手机号到用户ID的映射，方便通过手机号查找用户
	// 使用加密的手机号作为键，保护用户隐私
	if phone != "" {
		encryptedPhone, err := s.encryptService(phone)
		if err != nil {
			fmt.Printf("手机号加密错误: %s\n", err)
		} else {
			phoneKey := fmt.Sprintf("user:phone:%s", encryptedPhone)
			s.readWrite.SetRedis(phoneKey, userId, s.cacheExpireSeconds)
		}
	}

	// 邮箱到用户ID的映射，方便通过邮箱查找用户
	if email, ok := user["email"].(string); ok && email != "" {
		emailKey := fmt.Sprintf("user:email:%s", email)
		s.readWrite.SetRedis(emailKey, userId, s.cacheExpireSeconds)
	}

	return true, "用户信息缓存成功"
}

// GetUserById 通过用户ID获取缓存的用户信息
func (s *CommonUserCacheService) GetUserById(userId string) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("获取用户信息错误: %v\n", r)
		}
	}()

	redisKey := fmt.Sprintf("user:info:%s", userId)
	userInfoStr := s.readWrite.GetRedis(redisKey)

	if userInfoStr == "" {
		return nil
	}

	var userInfo map[string]interface{}
	err := json.Unmarshal([]byte(userInfoStr), &userInfo)
	if err != nil {
		fmt.Printf("解析用户信息错误: %s\n", err)
		return nil
	}

	return userInfo
}

// GetUserByPhone 通过手机号获取用户信息
func (s *CommonUserCacheService) GetUserByPhone(phone string) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("通过手机号获取用户信息错误: %v\n", r)
		}
	}()

	// 加密手机号
	encryptedPhone, err := s.encryptService(phone)
	if err != nil {
		fmt.Printf("手机号加密错误: %s\n", err)
		return nil
	}

	// 先获取用户ID
	phoneKey := fmt.Sprintf("user:phone:%s", encryptedPhone)
	userId := s.readWrite.GetRedis(phoneKey)

	if userId == "" {
		return nil
	}

	// 再获取用户详细信息
	return s.GetUserById(userId)
}

// GetUserByEmail 通过邮箱获取用户信息
func (s *CommonUserCacheService) GetUserByEmail(email string) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("通过邮箱获取用户信息错误: %v\n", r)
		}
	}()

	// 先获取用户ID
	emailKey := fmt.Sprintf("user:email:%s", email)
	userId := s.readWrite.GetRedis(emailKey)

	if userId == "" {
		return nil
	}

	// 再获取用户详细信息
	return s.GetUserById(userId)
}

// UpdateCachedUserInfo 更新Redis中缓存的用户信息
func (s *CommonUserCacheService) UpdateCachedUserInfo(userId string, updatedFields map[string]interface{}) (bool, string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("更新缓存的用户信息错误: %v\n", r)
		}
	}()

	// 获取现有用户信息
	userInfo := s.GetUserById(userId)

	if userInfo == nil {
		return false, "用户信息不存在"
	}

	// 如果要更新手机号，需要处理映射关系
	if newPhone, ok := updatedFields["phone"]; ok {
		oldPhone, _ := userInfo["phone"].(string)
		if newPhone != oldPhone {
			// 删除旧手机号映射
			if oldPhone != "" {
				oldEncryptedPhone, err := s.encryptService(oldPhone)
				if err == nil {
					oldPhoneKey := fmt.Sprintf("user:phone:%s", oldEncryptedPhone)
					s.readWrite.DeleteRedis(oldPhoneKey)
				} else {
					fmt.Printf("删除旧手机号映射错误: %s\n", err)
				}
			}

			// 创建新手机号映射
			if newPhoneStr, ok := newPhone.(string); ok && newPhoneStr != "" {
				newEncryptedPhone, err := s.encryptService(newPhoneStr)
				if err == nil {
					newPhoneKey := fmt.Sprintf("user:phone:%s", newEncryptedPhone)
					s.readWrite.SetRedis(newPhoneKey, userId, s.cacheExpireSeconds)
				} else {
					fmt.Printf("创建新手机号映射错误: %s\n", err)
				}
			}
		}
	}

	// 更新字段
	for key, value := range updatedFields {
		userInfo[key] = value
	}

	// 重新保存
	redisKey := fmt.Sprintf("user:info:%s", userId)
	jsonData, err := json.Marshal(userInfo)
	if err != nil {
		return false, fmt.Sprintf("更新用户信息错误: %s", err)
	}

	err = s.readWrite.SetRedis(redisKey, string(jsonData), s.cacheExpireSeconds)
	if err != nil {
		return false, fmt.Sprintf("更新用户信息错误: %s", err)
	}

	return true, "用户信息更新成功"
}

// UpdateUserInfo 对外暴露的用户信息更新方法，便于其他模块调用
func (s *CommonUserCacheService) UpdateUserInfo(userId string, updatedFields map[string]interface{}) error {
	if userId == "" {
		return fmt.Errorf("用户ID不能为空")
	}
	if len(updatedFields) == 0 {
		return fmt.Errorf("没有需要更新的字段")
	}

	success, message := s.UpdateCachedUserInfo(userId, updatedFields)
	if !success {
		return fmt.Errorf(message)
	}
	return nil
}

// RemoveUserCache 删除用户的缓存信息（如用户注销时）
func (s *CommonUserCacheService) RemoveUserCache(userId string) (bool, string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("删除用户缓存信息错误: %v\n", r)
		}
	}()

	// 获取用户信息，以便获取手机号和邮箱
	userInfo := s.GetUserById(userId)

	if userInfo == nil {
		return false, "用户信息不存在"
	}

	// 删除用户信息缓存
	redisKey := fmt.Sprintf("user:info:%s", userId)
	s.readWrite.DeleteRedis(redisKey)

	// 删除手机号映射
	if phone, ok := userInfo["phone"].(string); ok && phone != "" {
		encryptedPhone, err := s.encryptService(phone)
		if err == nil {
			phoneKey := fmt.Sprintf("user:phone:%s", encryptedPhone)
			s.readWrite.DeleteRedis(phoneKey)
		} else {
			fmt.Printf("删除手机号映射错误: %s\n", err)
		}
	}

	// 删除邮箱映射
	if email, ok := userInfo["email"].(string); ok && email != "" {
		emailKey := fmt.Sprintf("user:email:%s", email)
		s.readWrite.DeleteRedis(emailKey)
	}

	return true, "用户缓存信息已删除"
}
