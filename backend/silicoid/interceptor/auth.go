package interceptor

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// extractAuthToken 从请求中提取AuthToken
func (s *SilicoIDInterceptor) extractAuthToken(c *gin.Context) string {
	// 从Authorization头中提取
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	
	// 如果没有在Authorization头中找到，尝试从查询参数中提取
	authToken := c.Query("auth_token")
	if authToken != "" {
		return authToken
	}
	
	// 如果仍然没有找到，尝试从请求体中提取
	var data map[string]interface{}
	if err := c.ShouldBindJSON(&data); err == nil {
		if authToken, ok := data["auth_token"].(string); ok {
			return authToken
		}
	}
	
	return ""
}

// verifyAuthToken 验证AuthToken
// 注意：此函数只验证AuthToken是否有效，不检查余额。余额检查应在interceptor的HandleChatCompletions中进行。
func (s *SilicoIDInterceptor) verifyAuthToken(authToken string) (bool, string, string) {
	if authToken == "" {
		return false, "", "缺少AuthToken"
	}
	
	valid, payload := s.authTokenService.VerifyAuthToken(authToken)
	
	if !valid {
		errMsg, ok := payload.(string)
		if !ok {
			errMsg = "无效的AuthToken"
		}
		return false, "", errMsg
	}
	
	// 获取用户ID
	userId := payload.(map[string]interface{})["userId"].(string)
	
	// 余额检查应在interceptor的HandleChatCompletions中进行，不在这里检查
	return true, userId, ""
}

// verifyApiKey 验证API密钥
// 注意：此函数只验证API Key是否有效，不检查余额。余额检查应在interceptor的HandleChatCompletions中进行。
func (s *SilicoIDInterceptor) verifyApiKey(apiKey string) (bool, string, string) {
	if apiKey == "" {
		return false, "", "缺少API密钥"
	}
	
	valid, userId, errMsg := s.apiKeyManageService.VerifyApiKey(apiKey)
	
	if !valid {
		// 不使用类型断言，直接将错误消息格式化为字符串
		errorMessage := fmt.Sprintf("%v", errMsg)
		return false, "", errorMessage
	}
	
	// 余额检查应在interceptor的HandleChatCompletions中进行，不在这里检查
	return true, userId, ""
}

// checkUserAssets 检查用户资产（通过token或api_key）
// 如果api_key为空，使用token检查用户资产
// 如果api_key不为空，检查是否是平台API Key，如果是则查询对应的用户资产
func (s *SilicoIDInterceptor) checkUserAssets(userID string, apiKey string) (bool, error) {
	// 如果api_key为空，使用token检查用户资产
	if apiKey == "" {
		if userID == "" {
			return false, fmt.Errorf("用户ID为空")
		}
		
		// 查询用户资产
		userData := map[string]interface{}{
			"user_id": userID,
		}
		userAIPlatformData := s.fetchAiPlatformUserData(userData)
		
		// 提取资产信息
		if assets, ok := userAIPlatformData["assets"].(map[string]interface{}); ok {
			var giftedTokens int64 = 0
			var ownedTokens int64 = 0
			
			if gt, ok := assets["gifted_tokens"].(int64); ok {
				giftedTokens = gt
			} else if gt, ok := assets["gifted_tokens"].(int); ok {
				giftedTokens = int64(gt)
			}
			
			if ot, ok := assets["owned_tokens"].(int64); ok {
				ownedTokens = ot
			} else if ot, ok := assets["owned_tokens"].(int); ok {
				ownedTokens = int64(ot)
			}
			
			totalBalance := giftedTokens + ownedTokens
			logger.Printf("用户 %s token余额: gifted=%d, owned=%d, total=%d", userID, giftedTokens, ownedTokens, totalBalance)
			
			return totalBalance > 0, nil
		}
		
		return false, fmt.Errorf("用户 %s 没有资产信息", userID)
	}
	
	// 如果api_key不为空，检查是否是平台API Key
	if strings.HasPrefix(apiKey, "sk-potagi-") {
		// 验证API Key并获取用户ID
		valid, id, errorMessage := s.apiKeyManageService.VerifyApiKey(apiKey)
		if !valid {
			return false, fmt.Errorf("API密钥验证失败: %s", errorMessage)
		}
		
		// 查询用户资产
		userData := map[string]interface{}{
			"user_id": id,
		}
		userAIPlatformData := s.fetchAiPlatformUserData(userData)
		
		// 提取资产信息
		if assets, ok := userAIPlatformData["assets"].(map[string]interface{}); ok {
			var giftedTokens int64 = 0
			var ownedTokens int64 = 0
			
			if gt, ok := assets["gifted_tokens"].(int64); ok {
				giftedTokens = gt
			} else if gt, ok := assets["gifted_tokens"].(int); ok {
				giftedTokens = int64(gt)
			}
			
			if ot, ok := assets["owned_tokens"].(int64); ok {
				ownedTokens = ot
			} else if ot, ok := assets["owned_tokens"].(int); ok {
				ownedTokens = int64(ot)
			}
			
			totalBalance := giftedTokens + ownedTokens
			logger.Printf("用户 %s (通过API Key) token余额: gifted=%d, owned=%d, total=%d", id, giftedTokens, ownedTokens, totalBalance)
			
			return totalBalance > 0, nil
		}
		
		return false, fmt.Errorf("用户 %s (通过API Key) 没有资产信息", id)
	}
	
	// 如果不是平台API Key，返回true（使用用户自己的Key，不检查资产）
	return true, nil
}

func (s *SilicoIDInterceptor) fetchAiPlatformUserData(userData map[string]interface{}) map[string]interface{} {
	if s.aiPlatformDataService == nil {
		return map[string]interface{}{}
	}
	return s.aiPlatformDataService.GetUserAIBasicPlatformData(userData)
}