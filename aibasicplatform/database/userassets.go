package database

import "log"

// GetUserAIBasicPlatformData 获取aibasicplatform应用相关的用户数据，主要从aibasicplatform_user_api_keys和aibasicplatform_user_assets表获取数据
//
// 参数:
//   user: 用户信息映射，包含user_id等关键信息
//
// 返回:
//   包含用户API密钥和资产信息的映射
func (s *AIBasicPlatformDataService) GetUserAIBasicPlatformData(user map[string]interface{}) map[string]interface{} {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取用户AIBasicPlatform数据异常: %v", r)
		}
	}()

	userID, ok := user["user_id"].(string)
	if !ok {
		log.Printf("获取用户ID错误: 用户数据中没有有效的user_id")
		return map[string]interface{}{"message": "无法获取AIBasicPlatform应用数据"}
	}

	// 获取用户API密钥
	apiKeys := s.getUserApiKeys(userID)

	// 获取用户资产信息
	assets := s.getUserAssets(userID)

	// 如果用户没有资产记录，自动创建一个默认资产记录
	assetMap, ok := assets.(map[string]interface{})
	if !ok || assetMap == nil {
		success := s.createDefaultUserAssets(userID)
		if success {
			assets = s.getUserAssets(userID)
		}
	}

	// 返回完整数据，不做字段过滤，让调用方决定哪些字段返回给前端
	result := map[string]interface{}{
		"apiKeys": apiKeys,
	}

	// 如果assets是一个切片而且有元素，取第一个元素
	if assetSlice, ok := assets.([]map[string]interface{}); ok && len(assetSlice) > 0 {
		result["assets"] = assetSlice[0]
	} else {
		result["assets"] = assets
	}

	return result
}



// getUserAssets 获取用户的资产信息
func (s *AIBasicPlatformDataService) getUserAssets(userID string) interface{} {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取用户资产信息异常: %v", r)
		}
	}()

	query := `
		SELECT id, user_id, gifted_tokens, owned_tokens
		FROM aibasicplatform.aibasicplatform_user_assets
		WHERE user_id = ?
	`
	
	opResult := s.readWrite.QueryDb(query, userID)
	if !opResult.IsSuccess() {
		log.Printf("获取用户资产信息错误: %v", opResult.Error)
		return nil
	}
	
	result, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(result) == 0 {
		return nil
	}
	
	return result[0]
}

// createDefaultUserAssets 为用户创建默认资产记录
func (s *AIBasicPlatformDataService) createDefaultUserAssets(userID string) bool {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("创建用户默认资产记录异常: %v", r)
		}
	}()

	// 默认赠送10000个token
	defaultGiftedTokens := 10000
	
	insertQuery := `
		INSERT INTO aibasicplatform.aibasicplatform_user_assets 
		(user_id, gifted_tokens, owned_tokens) 
		VALUES (?, ?, 0)
	`
	
	opResult := s.readWrite.ExecuteDb(insertQuery, userID, defaultGiftedTokens)
	if !opResult.IsSuccess() {
		log.Printf("创建用户默认资产记录错误: %v", opResult.Error)
		return false
	}
	
	log.Printf("为用户%s创建默认资产记录，赠送%d个token", userID, defaultGiftedTokens)
	return true
}

