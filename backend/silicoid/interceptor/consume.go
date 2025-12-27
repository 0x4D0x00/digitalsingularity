package interceptor

// deductTokensIfNeeded 通用token扣除方法
// 检查是否需要扣除token，并在需要时执行扣除操作
func (s *SilicoIDInterceptor) deductTokensIfNeeded(data map[string]interface{}, userID string, requestID string) {
	// 解析并记录令牌使用情况
	usage, _ := data["usage"].(map[string]interface{})
	totalTokens := 0
	if usage != nil {
		if total, ok := usage["total_tokens"].(int); ok {
			totalTokens = total
		} else if totalFloat, ok := usage["total_tokens"].(float64); ok {
			totalTokens = int(totalFloat)
		}
	}

	// 只有在使用平台 Key 时才扣除用户令牌
	useUserKey, _ := data["_use_user_key"].(bool)
	if !useUserKey && totalTokens > 0 {
		success, remaining, err := s.apiKeyManageService.DeductTokens(userID, totalTokens)
		if !success {
			logger.Printf("[%s] 扣除令牌失败: %v", requestID, err)
		} else {
			logger.Printf("[%s] 扣除令牌成功，剩余: %d", requestID, remaining)
		}
	} else if useUserKey {
		logger.Printf("[%s] 使用用户自己的 Key，不扣除平台令牌", requestID)
	}
}

// DeductTokens 导出的通用token扣除函数
// 可以被任何需要扣除token的地方调用
// 参数说明：
// - tokenManager: token管理服务接口
// - data: 响应数据，包含usage信息
// - userID: 用户ID
// - requestID: 请求ID
// 返回值：
// - bool: 是否扣除成功
// - int: 剩余token数量（如果扣除成功）
// - error: 错误信息
type TokenManager interface {
	DeductTokens(userID string, tokens int) (bool, int, error)
}

func DeductTokens(tokenManager TokenManager, data map[string]interface{}, userID string, requestID string) (bool, int, error) {
	// 解析并记录令牌使用情况
	usage, _ := data["usage"].(map[string]interface{})
	totalTokens := 0
	if usage != nil {
		if total, ok := usage["total_tokens"].(int); ok {
			totalTokens = total
		} else if totalFloat, ok := usage["total_tokens"].(float64); ok {
			totalTokens = int(totalFloat)
		}
	}

	// 只有在使用平台 Key 时才扣除用户令牌
	useUserKey, _ := data["_use_user_key"].(bool)
	if !useUserKey && totalTokens > 0 {
		return tokenManager.DeductTokens(userID, totalTokens)
	} else if useUserKey {
		logger.Printf("[%s] 使用用户自己的 Key，不扣除平台令牌", requestID)
		return true, 0, nil // 不扣除token，视为成功
	}

	// 没有需要扣除的token，也视为成功
	return true, 0, nil
}
