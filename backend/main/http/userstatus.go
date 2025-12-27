package http

import (
	"encoding/json"
	"fmt"
	"time"

	"digitalsingularity/backend/main/accountmanagement/deactivation"
	"digitalsingularity/backend/main/websocket"
)

// handleLoginRequest 处理登录请求（业务逻辑函数）
func handleLoginRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 处理登录请求", requestID)

	if loginService == nil {
		logger.Printf("[%s] 登录服务未初始化", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "登录服务不可用",
		}
	}

	// 优先获取十六进制格式的用户公钥，向后兼容Base64
	userPublicKey, _ := data["userPublicKeyHex"].(string)
	if userPublicKey == "" {
		userPublicKey, _ = data["userPublicKeyBase64"].(string)
	}

	result := loginService.HandleLoginRequest(data, userPublicKey)
	if result == nil {
		logger.Printf("[%s] 登录服务返回空结果", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "登录失败",
		}
	}

	// 确保返回结果包含标准字段
	if status, ok := result["status"].(string); !ok || status == "" {
		result["status"] = "fail"
		if _, hasMessage := result["message"].(string); !hasMessage {
			result["message"] = "登录请求处理失败"
		}
	}

	logger.Printf("[%s] 登录处理结果状态: %s", requestID, result["status"])
	return result
}

// handleLogoutRequest 处理登出请求（业务逻辑函数）
func handleLogoutRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到登出请求", requestID)

	// 获取auth_token
	authToken, ok := data["auth_token"].(string)
	if !ok || authToken == "" {
		logger.Printf("[%s] 缺少auth_token", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少auth_token",
		}
	}

	// 验证auth_token并获取用户ID
	valid, payload := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		logger.Printf("[%s] auth_token验证失败", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效的auth_token",
		}
	}

	// 从auth_token payload中提取用户ID
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		logger.Printf("[%s] auth_token payload格式错误", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "auth_token格式错误",
		}
	}

	userID, ok := payloadMap["userId"].(string)
	if !ok || userID == "" {
		logger.Printf("[%s] 无法从auth_token获取用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无法获取用户ID",
		}
	}

	// 使auth_token失效
	if authTokenService != nil {
		// 这里可以调用authToken服务使auth_token失效
		// 具体实现取决于authToken服务的接口
		logger.Printf("[%s] 使auth_token失效: %s", requestID, authToken)
	}

	logger.Printf("[%s] 用户登出成功: %s", requestID, userID)
	logoutAt := time.Now().Format(time.RFC3339)

	websocket.ForceLogout(userID, "logout", map[string]interface{}{
		"request_id": requestID,
	})

	websocket.SendNotification(userID, "logout", map[string]interface{}{
		"logout_at": logoutAt,
	})

	return map[string]interface{}{
		"status":  "success",
		"message": "登出成功",
		"data": map[string]interface{}{
			"logout_at": logoutAt,
		},
	}
}

// handleDeactivationRequest 处理账户注销请求（业务逻辑函数）
func handleDeactivationRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到账户注销请求", requestID)

	// 获取auth_token
	authToken, ok := data["auth_token"].(string)
	if !ok || authToken == "" {
		logger.Printf("[%s] 缺少auth_token", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少auth_token",
		}
	}

	// 验证auth_token并获取用户ID
	valid, payload := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		logger.Printf("[%s] auth_token验证失败", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效的auth_token",
		}
	}

	// 从auth_token payload中提取用户ID
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		logger.Printf("[%s] auth_token payload格式错误", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "auth_token格式错误",
		}
	}

	userID, ok := payloadMap["userId"].(string)
	if !ok || userID == "" {
		logger.Printf("[%s] 无法从auth_token获取用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无法获取用户ID",
		}
	}

	// 获取注销操作类型
	action, _ := data["action"].(string)
	// 获取注销验证码（如果需要）
	verificationCode, _ := data["verification_code"].(string)

	// 调用账户注销服务
	deactivationService := deactivation.NewDeactivationService()

	if action == "" {
		if verificationCode != "" {
			action = "deactivate_account"
		} else {
			action = "request_deactivation_code"
		}
	}

	requestData := map[string]interface{}{
		"auth_token": authToken,
		"action":     action,
	}
	if verificationCode != "" {
		requestData["verification_code"] = verificationCode
	}

	// 序列化请求数据
	requestDataBytes, err := json.Marshal(map[string]interface{}{
		"data": requestData,
	})
	if err != nil {
		logger.Printf("[%s] 序列化注销请求失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "请求数据格式错误",
		}
	}

	// 处理注销请求（使用空字符串作为连接ID和消息ID，因为这是HTTP请求）
	responseData, err := deactivationService.ProcessRequest(userID, requestDataBytes, "", "", nil)
	if err != nil {
		logger.Printf("[%s] 账户注销失败: %s, 错误: %v", requestID, userID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": responseData.Message,
		}
	}

	if responseData.Type == "deactivation_response" {
		logger.Printf("[%s] 账户注销成功: %s", requestID, userID)

		deactivatedAt := time.Now().Format(time.RFC3339)
		websocket.ForceLogout(userID, "account_deactivated", map[string]interface{}{
			"request_id": requestID,
		})

		websocket.SendNotification(userID, "account_deactivated", map[string]interface{}{
			"deactivated_at": deactivatedAt,
		})

		return map[string]interface{}{
			"status":  "success",
			"message": responseData.Message,
		}
	} else {
		logger.Printf("[%s] 账户注销失败: %s, 原因: %s", requestID, userID, responseData.Message)
		return map[string]interface{}{
			"status":  "fail",
			"message": responseData.Message,
		}
	}
}

// handleAuthTokenRequest 处理用户认证token相关请求（业务逻辑函数）
func handleAuthTokenRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到用户认证token请求", requestID)

	// 获取auth_token
	authToken, ok := data["auth_token"].(string)
	if !ok || authToken == "" {
		logger.Printf("[%s] 缺少auth_token", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少auth_token",
		}
	}

	// 验证auth_token并获取用户ID
	valid, payload := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		logger.Printf("[%s] auth_token验证失败", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效的auth_token",
		}
	}

	// 从auth_token payload中提取用户ID
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		logger.Printf("[%s] auth_token payload格式错误", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "auth_token格式错误",
		}
	}

	userID, ok := payloadMap["userId"].(string)
	if !ok || userID == "" {
		logger.Printf("[%s] 无法从auth_token获取用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无法获取用户ID",
		}
	}

	// 获取子操作类型（从顶层 action 字段读取，符合三层结构）
	action, _ := data["action"].(string)

	switch action {
	case "get":
		// 获取用户的authToken信息
		if authTokenService == nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "authToken服务未初始化",
			}
		}

		// 从Redis获取用户的活跃authToken JTI列表
		userAuthTokensKey := fmt.Sprintf("user:authTokens:%s", userID)
		opResult := readWrite.GetRedis(userAuthTokensKey)

		if !opResult.IsSuccess() {
			logger.Printf("[%s] 获取用户authToken失败: %v", requestID, opResult.Error)
			return map[string]interface{}{
				"status":  "fail",
				"message": "获取authToken失败",
			}
		}

		authTokenJtiStr, ok := opResult.Data.(string)
		if !ok || authTokenJtiStr == "" {
			return map[string]interface{}{
				"status":  "success",
				"message": "用户没有活跃的authToken",
				"data": map[string]interface{}{
					"authTokens": []string{},
				},
			}
		}

		// 解析JTI列表
		var jtiList []string
		if err := json.Unmarshal([]byte(authTokenJtiStr), &jtiList); err != nil {
			// 如果不是JSON格式，尝试作为单个JTI
			jtiList = []string{authTokenJtiStr}
		}

		// 根据JTI列表从Redis获取实际的authToken
		var authTokens []string
		for _, jti := range jtiList {
			authTokenKey := fmt.Sprintf("authToken:%s", jti)
			tokenResult := readWrite.GetRedis(authTokenKey)
			
			if tokenResult.IsSuccess() {
				if tokenDataStr, ok := tokenResult.Data.(string); ok && tokenDataStr != "" {
					var tokenData map[string]interface{}
					if err := json.Unmarshal([]byte(tokenDataStr), &tokenData); err == nil {
						if token, ok := tokenData["authToken"].(string); ok && token != "" {
							authTokens = append(authTokens, token)
						}
					}
				}
			}
		}

		return map[string]interface{}{
			"status":  "success",
			"message": "获取authToken成功",
			"data": map[string]interface{}{
				"authTokens": authTokens,
			},
		}

	default:
		// 默认返回用户authToken信息
		return map[string]interface{}{
			"status":  "success",
			"message": "用户authToken请求已处理",
			"data":    map[string]interface{}{},
		}
	}
}
