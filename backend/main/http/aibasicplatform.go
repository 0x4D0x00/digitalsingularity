package http

import (
	"fmt"
	"sort"
	"strings"
	"time"

	aibasicplatformdatabase "digitalsingularity/backend/aibasicplatform/database"
)

var aiBasicPlatformService *aibasicplatformdatabase.AIBasicPlatformDataService

// handleAssetsTokensRequest 处理查询用户资产token余额请求（业务逻辑函数）
func handleAssetsTokensRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到查询用户资产token余额请求", requestID)

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

	if err := ensureAIBasicPlatformService(); err != nil {
		logger.Printf("[%s] 初始化数据服务失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "服务初始化失败",
		}
	}

	// 准备用户数据
	userData := map[string]interface{}{
		"user_id": userID,
	}

	// 获取用户资产信息
	userAIBasicPlatformData := aiBasicPlatformService.GetUserAIBasicPlatformData(userData)

	// 检查返回数据是否有效
	if userAIBasicPlatformData == nil {
		logger.Printf("[%s] 用户 %s 获取资产信息返回nil", requestID, userID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "获取用户资产信息失败",
		}
	}

	// 提取资产信息
	var giftedTokens int64 = 0
	var ownedTokens int64 = 0

	if assets, ok := userAIBasicPlatformData["assets"].(map[string]interface{}); ok && assets != nil {
		// 处理 gifted_tokens
		if gt, ok := assets["gifted_tokens"].(int64); ok {
			giftedTokens = gt
		} else if gt, ok := assets["gifted_tokens"].(int); ok {
			giftedTokens = int64(gt)
		} else if gt, ok := assets["gifted_tokens"].(float64); ok {
			giftedTokens = int64(gt)
		}

		// 处理 owned_tokens
		if ot, ok := assets["owned_tokens"].(int64); ok {
			ownedTokens = ot
		} else if ot, ok := assets["owned_tokens"].(int); ok {
			ownedTokens = int64(ot)
		} else if ot, ok := assets["owned_tokens"].(float64); ok {
			ownedTokens = int64(ot)
		}

		logger.Printf("[%s] 用户 %s token余额: gifted=%d, owned=%d", requestID, userID, giftedTokens, ownedTokens)
	} else {
		logger.Printf("[%s] 用户 %s 没有资产信息或资产信息格式错误", requestID, userID)
	}

	// 计算总余额
	totalBalance := giftedTokens + ownedTokens

	return map[string]interface{}{
		"status":  "success",
		"message": "查询成功",
		"data": map[string]interface{}{
			"balance":       totalBalance,
			"gifted_tokens": giftedTokens,
			"owned_tokens":  ownedTokens,
			"has_enough":    totalBalance > 0,
			"token_info": map[string]interface{}{
				"user_id": userID,
			},
		},
	}
}

// handleApiKeyManageRequest 处理API密钥管理请求（业务逻辑函数）
func handleApiKeyManageRequest(requestID, action string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到API密钥管理请求", requestID)

	// 获取auth_token
	authToken, ok := data["auth_token"].(string)
	if !ok || authToken == "" {
		logger.Printf("[%s] 缺少auth_token", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少auth_token",
		}
	}

	// 验证auth_token
	valid, _ := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		logger.Printf("[%s] auth_token验证失败", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效的auth_token",
		}
	}

	// 获取动作
	action = strings.TrimSpace(action)
	if action == "" {
		logger.Printf("[%s] 缺少API密钥动作", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少API密钥动作",
		}
	}

	logger.Printf("[%s] API密钥管理动作: %s", requestID, action)

	// 根据操作类型调用相应的处理函数
	switch action {
	case "list":
		return handleApiKeyListRequest(requestID, data)
	case "create":
		return handleApiKeyCreateRequest(requestID, data)
	case "delete":
		return handleApiKeyDeleteRequest(requestID, data)
	case "update_status":
		return handleApiKeyUpdateStatusRequest(requestID, data)
	default:
		logger.Printf("[%s] 未知的API密钥动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的API密钥动作: %s", action),
		}
	}
}

func handleSystemPromptOperations(requestID, action string, data map[string]interface{}) map[string]interface{} {
	action = strings.TrimSpace(action)
	if action == "" {
		logger.Printf("[%s] 缺少系统提示词子操作", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少系统提示词子操作",
		}
	}

	logger.Printf("[%s] 系统提示词动作: %s", requestID, action)

	switch action {
	case "list":
		return handleSystemPromptListRequest(requestID, data)
	case "refresh":
		return handleSystemPromptRefreshRequest(requestID, data)
	case "refresh_all":
		return handleSystemPromptRefreshAllRequest(requestID, data)
	case "get_all_roles":
		return handleGetAllRolesRequest(requestID, data)
	default:
		logger.Printf("[%s] 未知的系统提示词动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的系统提示词动作: %s", action),
		}
	}
}

func handleAppVersionOperationRequest(requestID, action string, data map[string]interface{}) map[string]interface{} {
	action = strings.TrimSpace(action)
	if action == "" {
		logger.Printf("[%s] 缺少应用版本子操作", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少应用版本子操作",
		}
	}

	logger.Printf("[%s] 应用版本动作: %s", requestID, action)

	switch action {
	case "fetch_latest_version":
		return handleFetchLatestAppVersionRequest(requestID, data)
	case "check_for_update":
		return handleCheckAppVersionUpdateRequest(requestID, data)
	default:
		logger.Printf("[%s] 未知的应用版本动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的应用版本动作: %s", action),
		}
	}
}

func handleTranslationOperations(requestID, action string, data map[string]interface{}) map[string]interface{} {
	action = strings.TrimSpace(action)
	if action == "" {
		logger.Printf("[%s] 缺少翻译子操作", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少翻译子操作",
		}
	}

	logger.Printf("[%s] 翻译动作: %s", requestID, action)

	switch action {
	case "get":
		return handleGetTranslationRequest(requestID, data)
	case "list":
		return handleGetTranslationsRequest(requestID, data)
	default:
		logger.Printf("[%s] 未知的翻译动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的翻译动作: %s", action),
		}
	}
}

// handleGetTranslationRequest 获取单条翻译文本，禁止在此函数中直接访问数据库
func handleGetTranslationRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	app := strings.TrimSpace(fmt.Sprintf("%v", data["app"]))
	category := strings.TrimSpace(fmt.Sprintf("%v", data["category"]))
	itemKey := strings.TrimSpace(fmt.Sprintf("%v", data["item_key"]))
	locale := strings.TrimSpace(fmt.Sprintf("%v", data["locale"]))

	if locale == "" {
		locale = "zh-CN"
	}

	if app == "" || category == "" || itemKey == "" {
		logger.Printf("[%s] 翻译参数缺失 app=%s category=%s item_key=%s", requestID, app, category, itemKey)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少 app/category/item_key 参数",
		}
	}

	translatedText, err := aibasicplatformdatabase.FetchTranslationText(app, category, itemKey, locale)
	if err != nil {
		logger.Printf("[%s] 获取翻译失败 app=%s category=%s item_key=%s locale=%s err=%v",
			requestID, app, category, itemKey, locale, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取翻译失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"app":             app,
			"category":        category,
			"item_key":        itemKey,
			"locale":          locale,
			"translated_text": translatedText,
		},
	}
}

// handleGetTranslationsRequest 获取翻译列表，底层数据通过 translation.go 封装的服务访问
func handleGetTranslationsRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	app := strings.TrimSpace(fmt.Sprintf("%v", data["app"]))
	category := strings.TrimSpace(fmt.Sprintf("%v", data["category"]))
	locale := strings.TrimSpace(fmt.Sprintf("%v", data["locale"]))

	if locale == "" {
		locale = "zh-CN"
	}

	if app == "" || category == "" {
		logger.Printf("[%s] 翻译列表参数缺失 app=%s category=%s", requestID, app, category)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少 app/category 参数",
		}
	}

	translations, err := aibasicplatformdatabase.FetchTranslationsDict(app, category, locale)
	if err != nil {
		logger.Printf("[%s] 获取翻译列表失败 app=%s category=%s locale=%s err=%v",
			requestID, app, category, locale, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取翻译列表失败: %v", err),
		}
	}

	keys := make([]string, 0, len(translations))
	for key := range translations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]map[string]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, map[string]string{
			"item_key":        key,
			"translated_text": translations[key],
		})
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"app":              app,
			"category":         category,
			"locale":           locale,
			"count":            len(items),
			"translations":     translations,
			"translation_list": items,
		},
	}
}

// handleFetchLatestAppVersionRequest 获取应用最新版本详情
func handleFetchLatestAppVersionRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到获取应用版本请求", requestID)

	if err := initAppVersionService(); err != nil {
		logger.Printf("[%s] 初始化应用版本服务失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("服务初始化失败: %v", err),
		}
	}

	// 获取平台参数
	platform, ok := data["platform"].(string)
	if !ok || platform == "" {
		logger.Printf("[%s] 缺少平台参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少platform参数",
		}
	}

	// 获取最新版本
	versionInfo, err := aiBasicPlatformService.GetLatestVersion(platform)
	if err != nil {
		logger.Printf("[%s] 获取应用版本失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取版本失败: %v", err),
		}
	}

	if versionInfo == nil {
		logger.Printf("[%s] 应用版本信息为空", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "应用版本信息不可用",
		}
	}

	logger.Printf("[%s] 成功获取应用版本: %s", requestID, versionInfo.Version)

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"version":               versionInfo.Version,
			"platform":              versionInfo.Platform,
			"is_force_update":       versionInfo.IsForceUpdate,
			"download_url":          versionInfo.DownloadURL,
			"file_size":             versionInfo.FileSize,
			"file_hash":             versionInfo.FileHash,
			"hash_type":             versionInfo.HashType,
			"release_notes":         versionInfo.ReleaseNotes,
			"min_supported_version": versionInfo.MinSupportedVersion,
			"created_at":            versionInfo.CreatedAt.Format(time.RFC3339),
			"updated_at":            versionInfo.UpdatedAt.Format(time.RFC3339),
		},
	}
}

// handleCheckAppVersionUpdateRequest 检查客户端是否需要更新
func handleCheckAppVersionUpdateRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到检查应用版本更新请求", requestID)

	if err := initAppVersionService(); err != nil {
		logger.Printf("[%s] 初始化应用版本服务失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("服务初始化失败: %v", err),
		}
	}

	// 获取客户端版本和平台
	clientVersion, ok := data["client_version"].(string)
	if !ok || clientVersion == "" {
		logger.Printf("[%s] 缺少客户端版本参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少client_version参数",
		}
	}

	platform, ok := data["platform"].(string)
	if !ok || platform == "" {
		logger.Printf("[%s] 缺少平台参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少platform参数",
		}
	}

	logger.Printf("[%s] 版本检查: 客户端版本=%s, 平台=%s", requestID, clientVersion, platform)

	// 比较版本
	needsUpdate, versionInfo, err := aiBasicPlatformService.CompareVersion(clientVersion, platform)
	if err != nil {
		logger.Printf("[%s] 版本比较失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("版本比较失败: %v", err),
		}
	}

	responseData := map[string]interface{}{
		"status":         "success",
		"needs_update":   needsUpdate,
		"client_version": clientVersion,
	}

	if versionInfo != nil {
		responseData["server_version"] = versionInfo.Version
		responseData["download_url"] = versionInfo.DownloadURL
		responseData["is_force_update"] = versionInfo.IsForceUpdate
		responseData["file_size"] = versionInfo.FileSize
		responseData["file_hash"] = versionInfo.FileHash
		responseData["hash_type"] = versionInfo.HashType
		responseData["release_notes"] = versionInfo.ReleaseNotes
		responseData["min_supported_version"] = versionInfo.MinSupportedVersion
	}

	logger.Printf("[%s] 版本检查完成: 客户端=%s, 服务器=%s, 需要更新=%v",
		requestID, clientVersion,
		func() string {
			if versionInfo != nil {
				return versionInfo.Version
			}
			return "未知"
		}(),
		needsUpdate)

	return responseData
}

// initAppVersionService 确保AIBasicPlatform数据服务可用于版本查询
func initAppVersionService() error {
	if err := ensureAIBasicPlatformService(); err != nil {
		return err
	}
	if aiBasicPlatformService == nil {
		return fmt.Errorf("AIBasicPlatform数据服务未就绪")
	}
	return nil
}

func ensureAIBasicPlatformService() error {
	if aiBasicPlatformService != nil {
		return nil
	}

	service := aibasicplatformdatabase.NewAIBasicPlatformDataService()
	if service == nil {
		return fmt.Errorf("AIBasicPlatform数据服务创建失败: service=nil")
	}

	aiBasicPlatformService = service
	return nil
}

// handleApiKeyListRequest 处理API密钥列表请求（业务逻辑函数）
func handleApiKeyListRequest(requestID string, data map[string]interface{}) map[string]interface{} {
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

	// 使用API密钥管理服务获取API密钥列表（只过滤 status=2 删除，0/1 全部返回）
	apiRequestData := map[string]interface{}{
		"type":       "list_api_keys",
		"auth_token": authToken,
	}
	apiResponse, err := apiKeyManageService.ProcessRequest(userID, map[string]interface{}{
		"data": apiRequestData,
	}, "", "")
	if err != nil {
		logger.Printf("[%s] 获取API密钥列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取API密钥列表失败: %v", err),
		}
	}

	if failResp := evaluateApiKeyServiceError(apiResponse); failResp != nil {
		return failResp
	}

	var keys []map[string]interface{}
	if dataMap, ok := apiResponse["data"].(map[string]interface{}); ok {
		switch raw := dataMap["keys"].(type) {
		case []map[string]interface{}:
			keys = raw
		case []interface{}:
			for _, item := range raw {
				if keyMap, ok := item.(map[string]interface{}); ok {
					keys = append(keys, keyMap)
				}
			}
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "获取API密钥列表成功",
		"data": map[string]interface{}{
			"list":  keys,
			"count": len(keys),
		},
	}
}

// handleApiKeyCreateRequest 处理API密钥创建请求（业务逻辑函数）
func handleApiKeyCreateRequest(requestID string, data map[string]interface{}) map[string]interface{} {
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

	// 获取API密钥名称
	apiKeyName, ok := data["name"].(string)
	if !ok || apiKeyName == "" {
		logger.Printf("[%s] API密钥名称不能为空", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "API密钥名称不能为空",
		}
	}

	// 使用API密钥管理服务创建API密钥
	createRequestData := map[string]interface{}{
		"type":       "create_api_key",
		"name":       apiKeyName,
		"auth_token": authToken,
	}

	response, err := apiKeyManageService.ProcessRequest(userID, map[string]interface{}{
		"data": createRequestData,
	}, "", "")
	if err != nil {
		logger.Printf("[%s] 创建API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("创建API密钥失败: %v", err),
		}
	}

	if failResp := evaluateApiKeyServiceError(response); failResp != nil {
		return failResp
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "API密钥创建成功",
		"data":    response,
	}
}

// handleApiKeyDeleteRequest 处理API密钥删除请求（业务逻辑函数）
func handleApiKeyDeleteRequest(requestID string, data map[string]interface{}) map[string]interface{} {
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

	// 获取API密钥ID（支持 id 或 key_id）
	apiKeyID, ok := data["id"].(string)
	if !ok || apiKeyID == "" {
		if kid, ok2 := data["key_id"].(string); ok2 {
			apiKeyID = kid
		}
	}
	if apiKeyID == "" {
		logger.Printf("[%s] API密钥ID不能为空", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "API密钥ID不能为空",
		}
	}

	// 使用API密钥管理服务删除API密钥（使用 key_id 字段）
	deleteRequestData := map[string]interface{}{
		"type":       "delete_api_key",
		"key_id":     apiKeyID,
		"auth_token": authToken,
	}

	response, err := apiKeyManageService.ProcessRequest(userID, map[string]interface{}{
		"data": deleteRequestData,
	}, "", "")
	if err != nil {
		logger.Printf("[%s] 删除API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("删除API密钥失败: %v", err),
		}
	}

	if failResp := evaluateApiKeyServiceError(response); failResp != nil {
		return failResp
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "API密钥删除成功",
		"data":    response,
	}
}

// handleApiKeyUpdateStatusRequest 处理API密钥状态更新请求（业务逻辑函数）
func handleApiKeyUpdateStatusRequest(requestID string, data map[string]interface{}) map[string]interface{} {
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

	// 获取API密钥ID（支持 id 或 key_id）
	apiKeyID, ok := data["id"].(string)
	if !ok || apiKeyID == "" {
		if kid, ok2 := data["key_id"].(string); ok2 {
			apiKeyID = kid
		}
	}
	if apiKeyID == "" {
		logger.Printf("[%s] API密钥ID不能为空", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "API密钥ID不能为空",
		}
	}

	// 获取目标状态（如 0=禁用, 1=启用, 2=删除 由服务层判定支持范围）
	// 允许 number 类型（float64）或字符串转为int
	var targetStatus interface{}
	if val, exists := data["status"]; exists {
		targetStatus = val
	} else if val, exists := data["new_status"]; exists {
		targetStatus = val
	}
	if targetStatus == nil {
		logger.Printf("[%s] 缺少status参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少status参数",
		}
	}

	// 直接透传给服务（服务内部做校验与转换，使用 key_id 字段）
	updateRequestData := map[string]interface{}{
		"type":       "update_api_key_status",
		"key_id":     apiKeyID,
		"status":     targetStatus,
		"auth_token": authToken,
	}

	response, err := apiKeyManageService.ProcessRequest(userID, map[string]interface{}{
		"data": updateRequestData,
	}, "", "")
	if err != nil {
		logger.Printf("[%s] 更新API密钥状态失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("更新API密钥状态失败: %v", err),
		}
	}

	if failResp := evaluateApiKeyServiceError(response); failResp != nil {
		return failResp
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "API密钥状态更新成功",
		"data":    response,
	}
}

func evaluateApiKeyServiceError(response map[string]interface{}) map[string]interface{} {
	if response == nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": "API密钥服务无响应",
		}
	}

	if respType, ok := response["type"].(string); ok && respType == "error" {
		message := "API密钥服务错误"
		if msg, ok := response["message"].(string); ok && msg != "" {
			message = msg
		}

		return map[string]interface{}{
			"status":  "fail",
			"message": message,
		}
	}

	return nil
}
