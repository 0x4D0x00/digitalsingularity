package interceptor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ErrorInfo 错误信息结构
type ErrorInfo struct {
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`
	Type       string `json:"type,omitempty"`
	Error      struct {
		Message string `json:"message,omitempty"`
		Type    string `json:"type,omitempty"`
	} `json:"error,omitempty"`
}

// IsAuthError 判断是否是认证错误
func (e *ErrorInfo) IsAuthError() bool {
	// 检查状态码是否为 401
	if e.StatusCode == 401 {
		return true
	}
	
	// 检查错误消息中是否包含认证相关的关键词
	message := strings.ToLower(e.Message)
	if e.Error.Message != "" {
		message += " " + strings.ToLower(e.Error.Message)
	}
	
	authKeywords := []string{
		"invalid api key",
		"incorrect api key",
		"authentication",
		"unauthorized",
		"invalid authentication",
		"api key is invalid",
		"api key无效",
		"认证",
		"授权",
	}
	
	for _, keyword := range authKeywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	
	return false
}

// IsErrorChunk 检测是否是错误消息块
func IsErrorChunk(chunk string) bool {
	// 检查是否是 SSE 格式的错误消息
	if strings.HasPrefix(chunk, "data: ") {
		data := strings.TrimPrefix(chunk, "data: ")
		data = strings.TrimSpace(data)
		
		// 检查是否包含 error 字段
		if strings.Contains(data, `"error"`) || strings.Contains(data, `"status_code"`) {
			return true
		}
	}
	
	// 检查是否是 JSON 格式的错误消息
	if strings.HasPrefix(strings.TrimSpace(chunk), "{") && 
		(strings.Contains(chunk, `"error"`) || strings.Contains(chunk, `"status_code"`)) {
		return true
	}
	
	return false
}

// ExtractErrorInfo 从 chunk 中提取错误信息
func ExtractErrorInfo(chunk string) *ErrorInfo {
	// 尝试从 SSE 格式中提取
	var data string
	if strings.HasPrefix(chunk, "data: ") {
		data = strings.TrimPrefix(chunk, "data: ")
		data = strings.TrimSpace(data)
	} else {
		data = strings.TrimSpace(chunk)
	}
	
	// 尝试解析 JSON
	var errInfo ErrorInfo
	if err := json.Unmarshal([]byte(data), &errInfo); err == nil {
		// 第一次解析成功，检查是否有有效的错误信息
		if errInfo.StatusCode != 0 || errInfo.Message != "" || errInfo.Type != "" ||
			errInfo.Error.Message != "" || errInfo.Error.Type != "" {
			return &errInfo
		}
	}
	
	// 如果第一次解析失败或没有有效信息，尝试嵌套的 error 字段
	var response struct {
		Error ErrorInfo `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &response); err == nil {
		if response.Error.StatusCode != 0 || response.Error.Message != "" || response.Error.Type != "" ||
			response.Error.Error.Message != "" || response.Error.Error.Type != "" {
			return &response.Error
		}
	}
	
	// 解析失败或没有有效错误信息
	return nil
}

// GenerateAuthErrorResponse 生成友好的认证错误响应
func GenerateAuthErrorResponse(modelName string) string {
	// 生成友好的错误消息，建议用户切换模型
	friendlyMessage := fmt.Sprintf(
		"当前模型的 API Key 已失效或无效，无法继续使用。\n\n"+
			"建议操作：\n"+
			"1. 请切换到其他可用模型\n"+
			"2. 或联系管理员更新 API Key\n\n"+
			"当前模型: %s",
		modelName,
	)
	
	// 构造 SSE 格式的错误响应
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"message": friendlyMessage,
			"type":    "authentication_error",
			"code":    "invalid_api_key",
		},
	}
	
	errorJSON, err := json.Marshal(errorResponse)
	if err != nil {
		// 如果序列化失败，返回简单的错误消息
		return fmt.Sprintf("data: {\"error\":{\"message\":\"%s\",\"type\":\"authentication_error\"}}\n\n", friendlyMessage)
	}
	
	return fmt.Sprintf("data: %s\n\n", string(errorJSON))
}

