package http

import (
	"fmt"
	"net/http"
	"strings"

	"digitalsingularity/backend/common/auth/smsverify"
	
	"digitalsingularity/backend/silicoid/interceptor"
)

// 处理请求的通用逻辑
// 如果返回 nil，表示已经直接写入响应（如流式响应），不需要再处理返回值
func processRequest(requestID string, data map[string]interface{}, nonceVerified bool, w http.ResponseWriter, r *http.Request) map[string]interface{} {
	// 获取请求类型
	requestType, ok := data["type"].(string)
	if !ok {
		logger.Printf("[%s] 缺少请求类型", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "无效请求",
		}
	}

	if !nonceVerified {
		// 验证nonce
		if err := VerifyNonce(requestID, data); err != nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "无效请求",
			}
		}
	}

	logger.Printf("[%s] 请求类型: %s", requestID, requestType)

	var result map[string]interface{}

	// 使用switch代替if-else嵌套结构
	switch requestType {
	case "command": // 命令类请求
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] 命令操作: %s", requestID, operation)

		switch operation {
		case "login":
			logger.Printf("[%s] 处理登录请求", requestID)
			return handleLoginRequest(requestID, data)

		case "send_sms_code":
			phone, ok := data["phone"].(string)
			if !ok {
				logger.Printf("[%s] 发送验证码请求缺少手机号", requestID)
				return map[string]interface{}{
					"status":  "fail",
					"message": "无效请求",
				}
			}

			logger.Printf("[%s] 处理发送验证码请求，手机号: %s", requestID, phone)
			smsService, _ := smsverify.NewSmsVerifyService()
			success, message := smsService.GenerateVerifyCode(phone)

			if success {
				logger.Printf("[%s] 验证码发送成功", requestID)
				result = map[string]interface{}{
					"status":  "success",
					"message": "验证码已发送",
				}
			} else {
				logger.Printf("[%s] 验证码发送失败: %s", requestID, message)
				result = map[string]interface{}{
					"status":  "fail",
					"message": message,
				}
			}

		case "logout":
			logger.Printf("[%s] 处理登出请求", requestID)
			result = map[string]interface{}{
				"status":  "fail",
				"message": "登出功能尚未实现",
			}

		default:
			logger.Printf("[%s] 未知的命令操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的命令操作: %s", operation),
			}
		}

	case "query": // 查询类请求
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] 查询操作: %s", requestID, operation)

		switch operation {
		case "read":
			logger.Printf("[%s] 处理读取查询", requestID)
			result = map[string]interface{}{
				"status":  "fail",
				"message": "查询功能尚未实现",
			}

		case "get_server_public_key":
			logger.Printf("[%s] 处理获取服务器公钥请求", requestID)
			keyResult := readWrite.GetServerPublicKey()
			if !keyResult.IsSuccess() || keyResult.Data == nil {
				logger.Printf("[%s] 服务器公钥读取失败: %v", requestID, keyResult.Error)
				result = map[string]interface{}{
					"status":  "fail",
					"message": "服务器公钥不可用",
				}
				break
			}

			serverPublicKey, ok := keyResult.Data.(string)
			if !ok || serverPublicKey == "" {
				logger.Printf("[%s] 服务器公钥类型错误", requestID)
				result = map[string]interface{}{
					"status":  "fail",
					"message": "服务器公钥不可用",
				}
				break
			}

			logger.Printf("[%s] 服务器公钥读取成功，长度: %d", requestID, len(serverPublicKey))
			result = map[string]interface{}{
				"status":          "success",
				"serverPublicKey": serverPublicKey,
			}

		default:
			logger.Printf("[%s] 未知的查询操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的查询操作: %s", operation),
			}
		}

	case "event": // 事件类请求
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] 事件操作: %s", requestID, operation)

		switch operation {
		case "notify":
			logger.Printf("[%s] 处理事件通知", requestID)
			result = map[string]interface{}{
				"status":  "fail",
				"message": "事件处理尚未实现",
			}

		case "collect_training_data":
			logger.Printf("[%s] 处理训练数据收集请求", requestID)

			if httpInterceptor == nil {
				logger.Printf("[%s] Silicoid拦截器未初始化", requestID)
				result = map[string]interface{}{
					"status":  "fail",
					"message": "MCP服务未初始化",
				}
				break
			}

			// 从请求中提取参数 - 数据可能直接在data中，而不是嵌套在training_data字段
			var trainingData map[string]interface{}

			// 首先尝试从training_data字段获取
			if td, ok := data["training_data"].(map[string]interface{}); ok {
				trainingData = td
			} else {
			// 如果没有training_data字段，使用整个data（除了type、operation、nonce和timestamp）
			trainingData = make(map[string]interface{})
			for key, value := range data {
				if key != "type" && key != "operation" && key != "nonce" && key != "timestamp" {
					trainingData[key] = value
				}
			}
			}

			if len(trainingData) == 0 {
				logger.Printf("[%s] 缺少训练数据", requestID)
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少训练数据",
				}
				break
			}

			logger.Printf("[%s] 提取的训练数据字段数量: %d", requestID, len(trainingData))

			// 通过拦截器调用服务端执行调用保存训练数据
			serverExecutorCall := &interceptor.ServerCall{
				Type:      "call",
				Name:      "training_save_dataset",
				Arguments: trainingData,
			}
			saveResult, err := httpInterceptor.ExecuteServerCall(nil, serverExecutorCall, requestID)
			if err != nil {
				logger.Printf("[%s] 保存训练数据失败: %v", requestID, err)
				result = map[string]interface{}{
					"status":  "fail",
					"message": fmt.Sprintf("保存训练数据失败: %v", err),
				}
				break
			}

			logger.Printf("[%s] 训练数据保存成功", requestID)
			result = map[string]interface{}{
				"status": "success",
				"data":   saveResult,
			}

		default:
			logger.Printf("[%s] 未知的事件操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的事件操作: %s", operation),
			}
		}

	case "api_key": // API密钥管理
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] API密钥操作: %s", requestID, operation)

		switch operation {
		case "list":
			result = handleApiKeyListRequest(requestID, data)
		case "create":
			result = handleApiKeyCreateRequest(requestID, data)
		case "delete":
			result = handleApiKeyDeleteRequest(requestID, data)
		case "update_status":
			result = handleApiKeyUpdateStatusRequest(requestID, data)
		default:
			logger.Printf("[%s] 未知的API密钥操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的API密钥操作: %s", operation),
			}
		}

	case "user_files": // 用户文件管理
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] 用户文件操作: %s", requestID, operation)

		switch operation {
		case "upload":
			result = handleFileUploadRequest(requestID, data)
		case "list":
			result = handleFileListRequest(requestID, data)
		case "delete":
			result = handleFileDeleteRequest(requestID, data)
		case "download":
			result = handleFileDownloadRequest(requestID, data)
		case "update_allowed_apps":
			result = handleFileUpdateAllowedAppsRequest(requestID, data)
		default:
			logger.Printf("[%s] 未知的用户文件操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的用户文件操作: %s", operation),
			}
		}

	case "communication_system": // 通信系统
		logger.Printf("[%s] 处理通信系统请求", requestID)
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}
		logger.Printf("[%s] 通信系统操作: %s", requestID, operation)
		switch operation {
		case "relationship_management":
			result = handleRelationshipManagementRequest(requestID, data)
		default:
			logger.Printf("[%s] 未知的通信系统操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的通信系统操作: %s", operation),
			}
		}
	case "user_info": // 用户信息管理
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] 用户信息操作: %s", requestID, operation)

		switch operation {
		case "modify_username":
			result = handleModifyUsernameRequest(requestID, data)
		case "modify_nickname":
			result = handleModifyNicknameRequest(requestID, data)
		case "modify_mobile":
			result = handleModifyMobileRequest(requestID, data)
		case "modify_email":
			result = handleModifyEmailRequest(requestID, data)
		case "modify_personal_assistant":
			result = handleModifyPersonalAssistantRequest(requestID, data)
		default:
			logger.Printf("[%s] 未知的用户信息操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的用户信息操作: %s", operation),
			}
		}

	case "user_status": // 用户状态管理
		operation, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少操作类型", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少操作类型",
			}
		}

		logger.Printf("[%s] 用户状态操作: %s", requestID, operation)

		switch operation {
		case "login":
			result = handleLoginRequest(requestID, data)
		case "logout":
			result = handleLogoutRequest(requestID, data)
		case "deactivation":
			result = handleDeactivationRequest(requestID, data)
		case "auth_token":
			result = handleAuthTokenRequest(requestID, data)
		default:
			logger.Printf("[%s] 未知的用户状态操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的用户状态操作: %s", operation),
			}
		}

	// security_check 模块已移除（不向后兼容）——相关请求将走默认分支返回错误
	case "ai_basic_platform": // AI基础平台信息
		operationVal, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少AI基础平台操作", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少AI基础平台操作",
			}
		}
		operation := strings.TrimSpace(operationVal)
		if operation == "" {
			logger.Printf("[%s] AI基础平台操作为空", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "无效的AI基础平台操作",
			}
		}

		actionVal, _ := data["action"].(string)
		action := strings.TrimSpace(actionVal)
		logger.Printf("[%s] AI基础平台操作: %s, 动作: %s", requestID, operation, action)

		switch operation {
		case "assets_tokens":
			result = handleAssetsTokensRequest(requestID, data)
		case "api_key_manage":
			if action == "" {
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少API密钥动作",
				}
				break
			}
			result = handleApiKeyManageRequest(requestID, action, data)
		case "system_prompt":
			if action == "" {
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少系统提示词动作",
				}
				break
			}
			result = handleSystemPromptOperations(requestID, action, data)
		case "app_version":
			if action == "" {
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少应用版本动作",
				}
				break
			}
			result = handleAppVersionOperationRequest(requestID, action, data)
		case "translation":
			if action == "" {
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少翻译动作",
				}
				break
			}
			result = handleTranslationOperations(requestID, action, data)
		default:
			logger.Printf("[%s] 未知的AI基础平台操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的AI基础平台操作: %s", operation),
			}
		}

	case "silicoid": // 硅基生命系统
		operationVal, ok := data["operation"].(string)
		if !ok {
			logger.Printf("[%s] 缺少Silicoid操作", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "缺少Silicoid操作",
			}
		}
		operation := strings.TrimSpace(operationVal)
		if operation == "" {
			logger.Printf("[%s] Silicoid操作为空", requestID)
			return map[string]interface{}{
				"status":  "fail",
				"message": "无效的Silicoid操作",
			}
		}

		actionVal, _ := data["action"].(string)
		action := strings.TrimSpace(actionVal)
		logger.Printf("[%s] 硅基生命系统操作: %s, 动作: %s", requestID, operation, action)

		switch operation {
		case "chat":
			// chat 请求需要直接写入HTTP响应（流式响应），不返回JSON map
			logger.Printf("[%s] 处理 silicoid chat 请求，直接写入响应", requestID)
			handleSilicoidChatCompletions(w, r, data, requestID)
			return nil // 返回 nil 表示已直接写入响应
		case "models":
			// models 请求返回结果给 handleEncryptedRequest 处理加密
			logger.Printf("[%s] 处理 silicoid models 请求", requestID)
			result = handleSilicoidModels(w, r, data, requestID)
		case "training_data":
			if action == "" {
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少训练数据动作",
				}
				break
			}
			result = handleSilicoidTrainingDataRequest(requestID, action, data)
		case "api_key_manage":
			// 硅基生命体后台API密钥管理，不给普通客户端提供
			if action == "" {
				result = map[string]interface{}{
					"status":  "fail",
					"message": "缺少API密钥动作",
				}
				break
			}
			result = handleSilicoidApiKeyManageRequest(requestID, action, data)
		default:
			logger.Printf("[%s] 未知的硅基生命系统操作: %s", requestID, operation)
			result = map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("未知的硅基生命系统操作: %s", operation),
			}
		}

	case "captcha": // 验证码发送
		logger.Printf("[%s] 处理验证码发送请求", requestID)
		result = handleCaptchaRequest(requestID, data)

	default:
		logger.Printf("[%s] 未知的请求类型: %s", requestID, requestType)
		result = map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的请求类型: %s", requestType),
		}
	}

	return result
}
