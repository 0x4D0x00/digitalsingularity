package http

import (
	"fmt"

	"digitalsingularity/backend/common/userinfostorage"
	relationshipmanagement "digitalsingularity/backend/communicationsystem/relationship_management"
	"digitalsingularity/backend/main/websocket"
)

// handleModifyUsernameRequest 处理修改用户名请求（业务逻辑函数）
func handleModifyUsernameRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到修改用户名请求", requestID)

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

	// 获取新用户名
	newUsername, ok := data["username"].(string)
	if !ok || newUsername == "" {
		logger.Printf("[%s] 缺少新用户名", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少新用户名",
		}
	}

	// 调用用户信息存储服务修改用户名
	userInfoService := userinfostorage.NewCommonUserCacheService(
		&rwAdapter{rw: readWrite},
		func(plainText string) (string, error) {
			return "", fmt.Errorf("加密功能未实现")
		},
		func(cipherText string) (string, error) {
			return "", fmt.Errorf("解密功能未实现")
		},
	)

	err := userInfoService.UpdateUserInfo(userID, map[string]interface{}{
		"username": newUsername,
	})
	if err != nil {
		logger.Printf("[%s] 修改用户名失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("修改用户名失败: %v", err),
		}
	}

	logger.Printf("[%s] 修改用户名成功: %s -> %s", requestID, userID, newUsername)

	notifyContactsAboutProfileChange(userID, "username_updated", map[string]interface{}{
		"username": newUsername,
	})

	return map[string]interface{}{
		"status":  "success",
		"message": "修改用户名成功",
		"data": map[string]interface{}{
			"user_id":  userID,
			"username": newUsername,
		},
	}
}

// handleModifyNicknameRequest 处理修改昵称请求（业务逻辑函数）
func handleModifyNicknameRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到修改昵称请求", requestID)

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

	// 获取新昵称
	newNickname, ok := data["nickname"].(string)
	if !ok || newNickname == "" {
		logger.Printf("[%s] 缺少新昵称", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少新昵称",
		}
	}

	// 调用用户信息存储服务修改昵称
	userInfoService := userinfostorage.NewCommonUserCacheService(
		&rwAdapter{rw: readWrite},
		func(plainText string) (string, error) {
			return "", fmt.Errorf("加密功能未实现")
		},
		func(cipherText string) (string, error) {
			return "", fmt.Errorf("解密功能未实现")
		},
	)

	err := userInfoService.UpdateUserInfo(userID, map[string]interface{}{
		"nickname": newNickname,
	})
	if err != nil {
		logger.Printf("[%s] 修改昵称失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("修改昵称失败: %v", err),
		}
	}

	logger.Printf("[%s] 修改昵称成功: %s -> %s", requestID, userID, newNickname)

	notifyContactsAboutProfileChange(userID, "nickname_updated", map[string]interface{}{
		"nickname": newNickname,
	})

	return map[string]interface{}{
		"status":  "success",
		"message": "修改昵称成功",
		"data": map[string]interface{}{
			"user_id":  userID,
			"nickname": newNickname,
		},
	}
}

// handleModifyMobileRequest 处理修改手机号请求（业务逻辑函数）
func handleModifyMobileRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到修改手机号请求", requestID)

	// 获取用户ID
	userID, ok := data["user_id"].(string)
	if !ok || userID == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 获取新手机号
	newMobile, ok := data["mobile"].(string)
	if !ok || newMobile == "" {
		logger.Printf("[%s] 缺少新手机号", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少新手机号",
		}
	}

	// 调用用户信息存储服务修改手机号
	userInfoService := userinfostorage.NewCommonUserCacheService(
		&rwAdapter{rw: readWrite},
		func(plainText string) (string, error) {
			return "", fmt.Errorf("加密功能未实现")
		},
		func(cipherText string) (string, error) {
			return "", fmt.Errorf("解密功能未实现")
		},
	)

	err := userInfoService.UpdateUserInfo(userID, map[string]interface{}{
		"mobile": newMobile,
	})
	if err != nil {
		logger.Printf("[%s] 修改手机号失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("修改手机号失败: %v", err),
		}
	}

	logger.Printf("[%s] 修改手机号成功: %s -> %s", requestID, userID, newMobile)

	notifyContactsAboutProfileChange(userID, "mobile_updated", map[string]interface{}{
		"mobile": newMobile,
	})

	return map[string]interface{}{
		"status":  "success",
		"message": "修改手机号成功",
		"data": map[string]interface{}{
			"user_id": userID,
			"mobile":  newMobile,
		},
	}
}

// handleModifyEmailRequest 处理修改邮箱请求（业务逻辑函数）
func handleModifyEmailRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到修改邮箱请求", requestID)

	// 获取用户ID
	userID, ok := data["user_id"].(string)
	if !ok || userID == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 获取新邮箱
	newEmail, ok := data["email"].(string)
	if !ok || newEmail == "" {
		logger.Printf("[%s] 缺少新邮箱", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少新邮箱",
		}
	}

	// 调用用户信息存储服务修改邮箱
	userInfoService := userinfostorage.NewCommonUserCacheService(
		&rwAdapter{rw: readWrite},
		func(plainText string) (string, error) {
			return "", fmt.Errorf("加密功能未实现")
		},
		func(cipherText string) (string, error) {
			return "", fmt.Errorf("解密功能未实现")
		},
	)

	err := userInfoService.UpdateUserInfo(userID, map[string]interface{}{
		"email": newEmail,
	})
	if err != nil {
		logger.Printf("[%s] 修改邮箱失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("修改邮箱失败: %v", err),
		}
	}

	logger.Printf("[%s] 修改邮箱成功: %s -> %s", requestID, userID, newEmail)

	notifyContactsAboutProfileChange(userID, "email_updated", map[string]interface{}{
		"email": newEmail,
	})

	return map[string]interface{}{
		"status":  "success",
		"message": "修改邮箱成功",
		"data": map[string]interface{}{
			"user_id": userID,
			"email":   newEmail,
		},
	}
}

func notifyContactsAboutProfileChange(userID, notificationType string, info map[string]interface{}) {
	contacts := collectUserContactIDs(userID)
	websocket.BroadcastUserInfoUpdate(userID, notificationType, contacts, info)
}

func collectUserContactIDs(userID string) []string {
	if userID == "" {
		return nil
	}

	seen := make(map[string]struct{})
	addContact := func(id string) {
		if id == "" || id == userID {
			return
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
		}
	}

	if friends, err := relationshipmanagement.GetFriends(userID, ""); err != nil {
		logger.Printf("[userinfo] 获取好友列表失败 (user_id=%s): %v", userID, err)
	} else {
		for _, friend := range friends {
			addContact(friend.FriendCommunicationID)
		}
	}

	if followers, err := relationshipmanagement.GetFollowers(userID); err != nil {
		logger.Printf("[userinfo] 获取粉丝列表失败 (user_id=%s): %v", userID, err)
	} else {
		for _, follower := range followers {
			addContact(follower.UserCommunicationID)
		}
	}

	if following, err := relationshipmanagement.GetFollowing(userID); err != nil {
		logger.Printf("[userinfo] 获取关注列表失败 (user_id=%s): %v", userID, err)
	} else {
		for _, followee := range following {
			addContact(followee.UserCommunicationID)
		}
	}

	contacts := make([]string, 0, len(seen))
	for id := range seen {
		contacts = append(contacts, id)
	}

	return contacts
}

// handleModifyPersonalAssistantRequest 处理修改私人助理开关请求
func handleModifyPersonalAssistantRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到修改私人助理开关请求", requestID)

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

	// 获取私人助理开关状态
	enabled, ok := data["personal_assistant_enabled"].(bool)
	if !ok {
		logger.Printf("[%s] 缺少或格式错误的personal_assistant_enabled参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或格式错误的personal_assistant_enabled参数",
		}
	}

	// 更新用户偏好设置
	preferenceValue := "0"
	if enabled {
		preferenceValue = "1"
	}

	query := `
		INSERT INTO common.user_preferences 
		(user_id, preference_key, preference_value, created_at, updated_at) 
		VALUES (?, 'personal_assistant_enabled', ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE 
		preference_value = VALUES(preference_value), 
		updated_at = NOW()
	`
	
	result := readWrite.ExecuteDb(query, userID, preferenceValue)
	if !result.IsSuccess() {
		logger.Printf("[%s] 更新私人助理开关失败: %v", requestID, result.Error)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("更新私人助理开关失败: %v", result.Error),
		}
	}

	logger.Printf("[%s] 修改私人助理开关成功: %s -> %v", requestID, userID, enabled)

	// 通知在线和离线好友
	notifyContactsAboutProfileChange(userID, "personal_assistant_updated", map[string]interface{}{
		"personal_assistant_enabled": enabled,
	})

	return map[string]interface{}{
		"status":  "success",
		"message": "修改私人助理开关成功",
		"data": map[string]interface{}{
			"personal_assistant_enabled": enabled,
		},
	}
}
