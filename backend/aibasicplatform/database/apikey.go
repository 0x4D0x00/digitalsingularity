package database

import (
	"log"

	"digitalsingularity/backend/common/auth/apikey"
)

// getUserApiKeys 获取用户的API密钥列表（使用ApiKeyService）
func (s *AIBasicPlatformDataService) getUserApiKeys(userID string) []map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取用户API密钥异常: %v", r)
		}
	}()

	// 使用ApiKeyService获取API密钥列表
	apiKeyService := apikey.NewApiKeyServiceWithDB(s.readWrite)
	success, keys, err := apiKeyService.ListApiKeys(userID, false) // false表示不包含已删除的密钥
	
	if !success || err != nil {
		log.Printf("获取用户API密钥错误: %v", err)
		return []map[string]interface{}{}
	}
	
	if keys == nil {
		return []map[string]interface{}{}
	}
	
	return keys
}