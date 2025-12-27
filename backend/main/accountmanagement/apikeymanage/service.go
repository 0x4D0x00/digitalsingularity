package apikeymanage

import (
	"encoding/json"
	"fmt"
	"log"
	
	"digitalsingularity/backend/common/auth/apikey"
	"digitalsingularity/backend/common/auth/tokenmanage"
)

// ApiKeyManageService API密钥管理服务，提供API密钥的创建、查询、删除等功能
type ApiKeyManageService struct {
	authTokenService *tokenmanage.CommonAuthTokenService
	apiKeyService *apikey.ApiKeyService
}

// NewApiKeyManageService 创建新的API密钥管理服务实例
func NewApiKeyManageService(authTokenService *tokenmanage.CommonAuthTokenService) *ApiKeyManageService {
	return &ApiKeyManageService{
		authTokenService: authTokenService,
		apiKeyService:    apikey.NewApiKeyService(),
	}
}

// VerifyApiKey 验证API密钥（供interceptor使用）
// 返回：是否有效，用户ID，错误消息
func (s *ApiKeyManageService) VerifyApiKey(apiKey string) (bool, string, error) {
	return s.apiKeyService.VerifyApiKey(apiKey)
}

// CheckUserTokens 检查用户令牌余额（供interceptor使用）
// 返回：是否有余额，剩余令牌，错误，令牌详情
func (s *ApiKeyManageService) CheckUserTokens(userID string) (bool, int, error, map[string]int) {
	return s.apiKeyService.CheckUserTokens(userID)
}

// DeductTokens 扣除用户令牌（供interceptor使用）
// 返回：是否成功，剩余令牌，错误
func (s *ApiKeyManageService) DeductTokens(userID string, tokens int) (bool, int, error) {
	return s.apiKeyService.DeductTokens(userID, tokens)
}

// ProcessRequest 处理API密钥管理请求
func (s *ApiKeyManageService) ProcessRequest(clientID string, messageData map[string]interface{}, connectionID string, msgID string) (map[string]interface{}, error) {
	log.Printf("[WS:%s:%s] 处理API密钥管理请求: %s", connectionID, msgID, jsonToString(messageData))

	// 验证基本消息结构
	data, ok := messageData["data"].(map[string]interface{})
	if !ok {
		return createErrorResponse("无效的请求格式"), nil
	}

	// 获取请求数据
	requestType, _ := data["type"].(string)
	authToken, _ := data["auth_token"].(string)

	// 验证authToken
	if authToken == "" {
		return createErrorResponse("authToken不能为空"), nil
	}

	valid, payload := s.authTokenService.VerifyAuthToken(authToken)
	if !valid {
		return createErrorResponse(fmt.Sprintf("无效的authToken: %v", payload)), nil
	}

	// 根据请求类型处理
	switch requestType {
	case "list_api_keys":
		return s.listApiKeys(clientID, connectionID, msgID)
	case "create_api_key":
		keyName, _ := data["name"].(string)
		return s.createApiKey(clientID, keyName, connectionID, msgID)
	case "delete_api_key":
		keyID, _ := data["key_id"].(string)
		return s.deleteApiKey(clientID, keyID, connectionID, msgID)
	case "update_api_key_status":
		keyID, _ := data["key_id"].(string)
		status, _ := data["status"].(float64)
		return s.updateApiKeyStatus(clientID, keyID, int(status), connectionID, msgID)
	default:
		return createErrorResponse(fmt.Sprintf("未知的API密钥操作类型: %s", requestType)), nil
	}
}

// listApiKeys 列出用户的API密钥
func (s *ApiKeyManageService) listApiKeys(userID string, connectionID string, msgID string) (map[string]interface{}, error) {
	log.Printf("[WS:%s:%s] 获取用户 %s 的API密钥列表", connectionID, msgID, userID)

	// 调用 apikey service 获取列表
	success, keys, err := s.apiKeyService.ListApiKeys(userID, false)
	if !success {
		log.Printf("[WS:%s:%s] 获取API密钥列表失败: %v", connectionID, msgID, err)
		return createErrorResponse(fmt.Sprintf("获取API密钥列表失败: %v", err)), nil
	}

	log.Printf("[WS:%s:%s] 获取到 %d 个API密钥", connectionID, msgID, len(keys))

	return map[string]interface{}{
		"type": "api_key_list_response",
		"data": map[string]interface{}{
			"keys": keys,
		},
	}, nil
}

// createApiKey 创建新的API密钥
func (s *ApiKeyManageService) createApiKey(userID string, keyName string, connectionID string, msgID string) (map[string]interface{}, error) {
	log.Printf("[WS:%s:%s] 用户 %s 创建新的API密钥: %s", connectionID, msgID, userID, keyName)

	// 验证输入
	if keyName == "" {
		return createErrorResponse("API密钥名称不能为空"), nil
	}

	// 调用 apikey service 创建API密钥（带数量限制）
	const maxApiKeys = 3
	success, newKey, err := s.apiKeyService.CreateApiKeyWithLimit(userID, keyName, maxApiKeys)
	if !success {
		log.Printf("[WS:%s:%s] 创建API密钥失败: %v", connectionID, msgID, err)
		return createErrorResponse(err.Error()), nil
	}

	log.Printf("[WS:%s:%s] API密钥创建成功", connectionID, msgID)

	// 返回API密钥对象
	return newKey, nil
}

// deleteApiKey 删除API密钥
func (s *ApiKeyManageService) deleteApiKey(userID string, keyID string, connectionID string, msgID string) (map[string]interface{}, error) {
	log.Printf("[WS:%s:%s] 用户 %s 删除API密钥ID: %s", connectionID, msgID, userID, keyID)

	// 验证输入
	if keyID == "" {
		return createErrorResponse("key_id不能为空"), nil
	}

	// 调用 apikey service 删除API密钥
	success, err := s.apiKeyService.DeleteApiKey(userID, keyID)
	if !success {
		log.Printf("[WS:%s:%s] 删除API密钥失败: %v", connectionID, msgID, err)
		return createErrorResponse(err.Error()), nil
	}

	log.Printf("[WS:%s:%s] API密钥删除成功，ID: %s", connectionID, msgID, keyID)

	return map[string]interface{}{
		"type": "delete_api_key_response",
		"data": map[string]interface{}{
			"key_id":  keyID,
			"success": true,
		},
	}, nil
}

// updateApiKeyStatus 更新API密钥状态（禁用/启用）
func (s *ApiKeyManageService) updateApiKeyStatus(userID string, keyID string, status int, connectionID string, msgID string) (map[string]interface{}, error) {
	log.Printf("[WS:%s:%s] 用户 %s 更新API密钥状态，ID: %s, status: %d", connectionID, msgID, userID, keyID, status)

	// 验证输入
	if keyID == "" {
		return createErrorResponse("key_id不能为空"), nil
	}

	// 验证status值（只允许0或1）
	if status != 0 && status != 1 {
		return createErrorResponse("status值无效（只允许0或1）"), nil
	}

	// 调用 apikey service 更新状态
	success, err := s.apiKeyService.UpdateApiKeyStatus(userID, keyID, status)
	if !success {
		log.Printf("[WS:%s:%s] 更新API密钥状态失败: %v", connectionID, msgID, err)
		return createErrorResponse(err.Error()), nil
	}

	statusText := "禁用"
	if status == 1 {
		statusText = "启用"
	}

	log.Printf("[WS:%s:%s] API密钥状态更新成功，ID: %s, status: %d (%s)", connectionID, msgID, keyID, status, statusText)

	return map[string]interface{}{
		"type": "update_api_key_status_response",
		"data": map[string]interface{}{
			"key_id":  keyID,
			"status":  status,
			"success": true,
			"message": fmt.Sprintf("API密钥已%s", statusText),
		},
	}, nil
}

// 辅助函数

// createErrorResponse 创建错误响应
func createErrorResponse(message string) map[string]interface{} {
	return map[string]interface{}{
		"type":    "error",
		"message": message,
	}
}

// jsonToString 将JSON对象转换为字符串
func jsonToString(obj interface{}) string {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("%v", obj)
	}
	return string(jsonBytes)
}

// 单例实例
var ApiKeyManageServiceInstance *ApiKeyManageService

// InitApiKeyManageService 初始化API密钥管理服务单例
func InitApiKeyManageService(authTokenService *tokenmanage.CommonAuthTokenService) {
	if ApiKeyManageServiceInstance == nil {
		ApiKeyManageServiceInstance = NewApiKeyManageService(authTokenService)
	}
} 