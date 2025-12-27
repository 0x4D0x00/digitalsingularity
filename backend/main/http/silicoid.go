package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	silicoidhttp "digitalsingularity/backend/silicoid/http"
)

var (
	silicoidServiceInitOnce sync.Once
	silicoidServiceInitErr  error
)

// 处理Silicoid聊天完成请求（接收已解密的数据）
func handleSilicoidChatCompletions(w http.ResponseWriter, r *http.Request, data map[string]interface{}, requestID string) {
	logger.Printf("[%s] 准备调用Silicoid聊天完成服务", requestID)

	// 提取实际的数据（data字段可能是JSON字符串或对象）
	var actualData map[string]interface{}
	if dataStr, ok := data["data"].(string); ok {
		// data 是字符串，需要解析
		if err := json.Unmarshal([]byte(dataStr), &actualData); err != nil {
			logger.Printf("[%s] 解析data字段失败: %v", requestID, err)
			respondWithError(w, "无效的请求数据格式", http.StatusBadRequest)
			return
		}
	} else if dataObj, ok := data["data"].(map[string]interface{}); ok {
		// data 是对象，直接使用
		actualData = dataObj
	} else {
		// data 字段不存在或格式不对，尝试直接使用 data 本身
		actualData = data
	}

	// 将实际数据序列化为JSON，准备传给silicoid服务
	jsonData, err := json.Marshal(actualData)
	if err != nil {
		logger.Printf("[%s] 序列化数据失败: %v", requestID, err)
		respondWithError(w, "内部服务器错误", http.StatusInternalServerError)
		return
	}

	// 创建新的请求体，包含实际的请求数据
	r.Body = io.NopCloser(strings.NewReader(string(jsonData)))
	r.ContentLength = int64(len(jsonData))

	// 调用Silicoid服务（返回不加密的响应）
	silicoidhttp.SilicoidChatCompletions(w, r, data, requestID)
}

// responseRecorder 用于捕获HTTP响应
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
	headerWritten bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.headerWritten {
		rr.statusCode = code
		rr.headerWritten = true
	}
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.headerWritten {
		rr.statusCode = http.StatusOK
		rr.headerWritten = true
	}
	rr.body = append(rr.body, b...)
	return len(b), nil
}

// 处理Silicoid模型列表请求（接收已解密的数据）
// 返回结果给 handleEncryptedRequest 处理加密
func handleSilicoidModels(w http.ResponseWriter, r *http.Request, data map[string]interface{}, requestID string) map[string]interface{} {
	logger.Printf("[%s] 准备调用Silicoid模型列表服务", requestID)

	// 提取实际的数据（data字段可能是JSON字符串或对象）
	var actualData map[string]interface{}
	if dataStr, ok := data["data"].(string); ok {
		// data 是字符串，需要解析
		if err := json.Unmarshal([]byte(dataStr), &actualData); err != nil {
			logger.Printf("[%s] 解析data字段失败: %v", requestID, err)
			return map[string]interface{}{
				"status":  "fail",
				"message": "无效的请求数据格式",
			}
		}
	} else if dataObj, ok := data["data"].(map[string]interface{}); ok {
		// data 是对象，直接使用
		actualData = dataObj
	} else {
		// data 字段不存在或格式不对，尝试直接使用 data 本身
		actualData = data
	}

	// 将实际数据序列化为JSON，准备传给silicoid服务
	jsonData, err := json.Marshal(actualData)
	if err != nil {
		logger.Printf("[%s] 序列化数据失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "内部服务器错误",
		}
	}

	// 创建新的请求体，包含实际的请求数据
	r.Body = io.NopCloser(strings.NewReader(string(jsonData)))
	r.ContentLength = int64(len(jsonData))

	// 创建响应记录器来捕获Silicoid服务的响应
	recorder := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           []byte{},
		headerWritten:  false,
	}

	// 调用Silicoid服务，响应会被记录到recorder中
	silicoidhttp.SilicoidModels(recorder, r)

	// 检查响应状态码
	if recorder.statusCode != http.StatusOK {
		logger.Printf("[%s] Silicoid服务返回错误状态码: %d", requestID, recorder.statusCode)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("Silicoid服务返回错误: HTTP %d", recorder.statusCode),
		}
	}

	// 解析Silicoid服务返回的响应（OpenAI格式）
	var silicoidResponse map[string]interface{}
	if err := json.Unmarshal(recorder.body, &silicoidResponse); err != nil {
		logger.Printf("[%s] 解析Silicoid响应失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "内部服务器错误",
		}
	}

	// 返回OpenAI格式的响应，让 handleEncryptedRequest 处理加密
	logger.Printf("[%s] Silicoid模型列表获取成功，返回OpenAI格式", requestID)
	return map[string]interface{}{
		"status": "success",
		"data":   silicoidResponse, // OpenAI格式：{object: "list", data: [...]}
	}
}

// ============ Silicoid Unified Operations ============

// handleSilicoidTrainingDataRequest routes Silicoid training data operations to the trainingDataService.
func handleSilicoidTrainingDataRequest(requestID, action string, data map[string]interface{}) map[string]interface{} {
	if trainingDataService == nil {
		logger.Printf("[%s] 训练数据服务未初始化", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "训练数据服务未初始化",
		}
	}

	action = strings.TrimSpace(action)
	if action == "" {
		logger.Printf("[%s] 缺少训练数据动作", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少训练数据动作",
		}
	}

	logger.Printf("[%s] Silicoid训练数据动作: %s", requestID, action)

	switch action {
	case "create":
		return trainingDataService.HandleCreate(data)
	case "createBatch":
		return trainingDataService.HandleCreateBatch(data)
	case "get":
		return trainingDataService.HandleGet(data)
	case "list":
		return trainingDataService.HandleList(data)
	case "update":
		return trainingDataService.HandleUpdate(data)
	case "updateBatch":
		return trainingDataService.HandleUpdateBatch(data)
	case "delete":
		return trainingDataService.HandleDelete(data)
	case "deleteBatch":
		return trainingDataService.HandleDeleteBatch(data)
	case "hardDelete":
		return trainingDataService.HandleHardDelete(data)
	case "hardDeleteBatch":
		return trainingDataService.HandleHardDeleteBatch(data)
	default:
		logger.Printf("[%s] 未知的训练数据动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的训练数据动作: %s", action),
		}
	}
}

// handleSilicoidApiKeyManageRequest routes Silicoid API key management operations to the Silicoid data service.
func handleSilicoidApiKeyManageRequest(requestID, action string, data map[string]interface{}) map[string]interface{} {
	if silicoidService == nil {
		logger.Printf("[%s] Silicoid数据服务未初始化", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "Silicoid数据服务未初始化",
		}
	}

	action = strings.TrimSpace(action)
	if action == "" {
		logger.Printf("[%s] 缺少API密钥动作", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少API密钥动作",
		}
	}

	logger.Printf("[%s] Silicoid API密钥动作: %s", requestID, action)

	switch action {
	case "list", "listAll":
		return handleSilicoidApiKeyList(requestID, data)
	case "listByModel", "listByCode":
		return handleSilicoidApiKeyListByModel(requestID, data)
	case "get":
		return handleSilicoidApiKeyGet(requestID, data)
	case "getAvailable", "getActiveKey":
		return handleSilicoidApiKeyGetAvailable(requestID, data)
	case "create":
		return handleSilicoidApiKeyCreate(requestID, data)
	case "update":
		return handleSilicoidApiKeyUpdate(requestID, data)
	case "updateStatus":
		return handleSilicoidApiKeyUpdateStatus(requestID, data)
	case "delete":
		return handleSilicoidApiKeyDelete(requestID, data)
	case "hardDelete":
		return handleSilicoidApiKeyHardDelete(requestID, data)
	case "updateUsage":
		return handleSilicoidApiKeyUpdateUsage(requestID, data)
	default:
		logger.Printf("[%s] 未知的Silicoid API密钥动作: %s", requestID, action)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("未知的Silicoid API密钥动作: %s", action),
		}
	}
}

func handleSilicoidApiKeyList(requestID string, data map[string]interface{}) map[string]interface{} {
	modelCode, _ := data["model_code"].(string)

	var statusPtr *int
	if statusVal, ok := parseIntFromInterface(data["status"]); ok {
		status := statusVal
		statusPtr = &status
	}

	orderBy, _ := data["order_by"].(string)

	rows, err := silicoidService.GetAllModelApiKeys(modelCode, statusPtr, orderBy)
	if err != nil {
		logger.Printf("[%s] 获取API密钥列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取API密钥列表失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"list":  rows,
			"count": len(rows),
		},
	}
}

func handleSilicoidApiKeyListByModel(requestID string, data map[string]interface{}) map[string]interface{} {
	modelCode, _ := data["model_code"].(string)
	if strings.TrimSpace(modelCode) == "" {
		logger.Printf("[%s] 缺少model_code", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少model_code",
		}
	}

	rows, err := silicoidService.GetModelAPIKeys(modelCode)
	if err != nil {
		logger.Printf("[%s] 根据model_code获取API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("根据model_code获取API密钥失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"list":  rows,
			"count": len(rows),
		},
	}
}

func handleSilicoidApiKeyGet(requestID string, data map[string]interface{}) map[string]interface{} {
	id, ok := parseIntFromInterface(data["id"])
	if !ok {
		logger.Printf("[%s] 缺少或无效的id", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的id",
		}
	}

	info, err := silicoidService.GetModelApiKeyByID(id)
	if err != nil {
		logger.Printf("[%s] 获取API密钥详情失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取API密钥详情失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data":   info,
	}
}

func handleSilicoidApiKeyGetAvailable(requestID string, data map[string]interface{}) map[string]interface{} {
	modelCode, _ := data["model_code"].(string)
	if strings.TrimSpace(modelCode) == "" {
		logger.Printf("[%s] 缺少model_code", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少model_code",
		}
	}

	info, err := silicoidService.GetAPIKeyByModelCode(modelCode)
	if err != nil {
		logger.Printf("[%s] 获取可用API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取可用API密钥失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data":   info,
	}
}

func handleSilicoidApiKeyCreate(requestID string, data map[string]interface{}) map[string]interface{} {
	modelCode, _ := data["model_code"].(string)
	if strings.TrimSpace(modelCode) == "" {
		logger.Printf("[%s] 缺少model_code", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少model_code",
		}
	}

	apiKey, _ := data["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		logger.Printf("[%s] 缺少api_key", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少api_key",
		}
	}

	var keyNamePtr *string
	if keyName, ok := data["key_name"].(string); ok && keyName != "" {
		name := keyName
		keyNamePtr = &name
	}

	var (
		statusPtr       *int
		priorityPtr     *int
		rateLimitPerMin *int
		rateLimitPerDay *int
		expiresAtPtr    *string
	)

	if val, ok := parseIntFromInterface(data["status"]); ok {
		status := val
		statusPtr = &status
	}
	if val, ok := parseIntFromInterface(data["priority"]); ok {
		priority := val
		priorityPtr = &priority
	}
	if val, ok := parseIntFromInterface(data["rate_limit_per_min"]); ok {
		tmp := val
		rateLimitPerMin = &tmp
	}
	if val, ok := parseIntFromInterface(data["rate_limit_per_day"]); ok {
		tmp := val
		rateLimitPerDay = &tmp
	}
	if expiresAt, ok := data["expires_at"].(string); ok && strings.TrimSpace(expiresAt) != "" {
		exp := expiresAt
		expiresAtPtr = &exp
	}

	id, err := silicoidService.CreateModelApiKey(
		modelCode,
		apiKey,
		keyNamePtr,
		statusPtr,
		priorityPtr,
		rateLimitPerMin,
		rateLimitPerDay,
		expiresAtPtr,
	)
	if err != nil {
		logger.Printf("[%s] 创建API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("创建API密钥失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"id": id,
		},
	}
}

func handleSilicoidApiKeyUpdate(requestID string, data map[string]interface{}) map[string]interface{} {
	id, ok := parseIntFromInterface(data["id"])
	if !ok {
		logger.Printf("[%s] 更新API密钥缺少id", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的id",
		}
	}

	updates := make(map[string]interface{})

	if apiKey, ok := data["api_key"].(string); ok && apiKey != "" {
		updates["api_key"] = apiKey
	}
	if keyName, ok := data["key_name"].(string); ok {
		if keyName == "" {
			updates["key_name"] = nil
		} else {
			updates["key_name"] = keyName
		}
	}
	if status, ok := parseIntFromInterface(data["status"]); ok {
		updates["status"] = status
	}
	if priority, ok := parseIntFromInterface(data["priority"]); ok {
		updates["priority"] = priority
	}
	if rateLimitPerMin, ok := parseIntFromInterface(data["rate_limit_per_min"]); ok {
		updates["rate_limit_per_min"] = rateLimitPerMin
	}
	if rateLimitPerDay, ok := parseIntFromInterface(data["rate_limit_per_day"]); ok {
		updates["rate_limit_per_day"] = rateLimitPerDay
	}
	if expiresAt, ok := data["expires_at"].(string); ok {
		if strings.TrimSpace(expiresAt) == "" {
			updates["expires_at"] = nil
		} else {
			updates["expires_at"] = expiresAt
		}
	}

	if len(updates) == 0 {
		logger.Printf("[%s] 没有可更新的字段", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "没有可更新的字段",
		}
	}

	if err := silicoidService.UpdateModelApiKey(id, updates); err != nil {
		logger.Printf("[%s] 更新API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("更新API密钥失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "更新成功",
	}
}

func handleSilicoidApiKeyUpdateStatus(requestID string, data map[string]interface{}) map[string]interface{} {
	id, ok := parseIntFromInterface(data["id"])
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的id",
		}
	}

	status, ok := parseIntFromInterface(data["status"])
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的status",
		}
	}

	if err := silicoidService.UpdateModelApiKey(id, map[string]interface{}{"status": status}); err != nil {
		logger.Printf("[%s] 更新API密钥状态失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("更新API密钥状态失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "状态更新成功",
	}
}

func handleSilicoidApiKeyDelete(requestID string, data map[string]interface{}) map[string]interface{} {
	id, ok := parseIntFromInterface(data["id"])
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的id",
		}
	}

	if err := silicoidService.DeleteModelApiKey(id); err != nil {
		logger.Printf("[%s] 删除API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("删除API密钥失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "删除成功",
	}
}

func handleSilicoidApiKeyHardDelete(requestID string, data map[string]interface{}) map[string]interface{} {
	id, ok := parseIntFromInterface(data["id"])
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的id",
		}
	}

	if err := silicoidService.HardDeleteModelApiKey(id); err != nil {
		logger.Printf("[%s] 硬删除API密钥失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("硬删除API密钥失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "硬删除成功",
	}
}

func handleSilicoidApiKeyUpdateUsage(requestID string, data map[string]interface{}) map[string]interface{} {
	id, ok := parseIntFromInterface(data["id"])
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的id",
		}
	}

	success, ok := parseBoolFromInterface(data["success"])
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少或无效的success字段",
		}
	}

	var failReasonPtr *string
	if failReason, ok := data["fail_reason"].(string); ok && failReason != "" {
		fr := failReason
		failReasonPtr = &fr
	}

	if err := silicoidService.UpdateModelApiKeyUsage(id, success, failReasonPtr); err != nil {
		logger.Printf("[%s] 更新API密钥使用统计失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("更新API密钥使用统计失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "更新使用统计成功",
	}
}

func parseIntFromInterface(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		if val, err := v.Int64(); err == nil {
			return int(val), true
		}
	case string:
		val := strings.TrimSpace(v)
		if val == "" {
			return 0, false
		}
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func parseBoolFromInterface(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		switch lower {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		}
	case float32:
		return v != 0, true
	case float64:
		return v != 0, true
	case int:
		return v != 0, true
	case int8:
		return v != 0, true
	case int16:
		return v != 0, true
	case int32:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint:
		return v != 0, true
	case uint8:
		return v != 0, true
	case uint16:
		return v != 0, true
	case uint32:
		return v != 0, true
	case uint64:
		return v != 0, true
	}
	return false, false
}
