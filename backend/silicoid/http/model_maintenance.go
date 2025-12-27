package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"

	"digitalsingularity/backend/silicoid/models/maintenance"
)

var maintenanceLogger = log.New(log.Writer(), "[ModelMaintenance] ", log.LstdFlags)

// 全局维护服务实例
var maintenanceService *maintenance.MaintenanceService

// 初始化维护服务
func init() {
	maintenanceService = maintenance.NewMaintenanceService()
	maintenanceLogger.Printf("✅ 模型维护服务初始化完成")
}

// handleSyncProviderModels 同步指定提供商的模型列表
// 路由: POST /v1/models/sync/{provider}
func handleSyncProviderModels(w http.ResponseWriter, r *http.Request) {
	// 从 URL 路径参数获取提供商名称
	vars := mux.Vars(r)
	provider := vars["provider"]
	
	if provider == "" {
		respondWithError(w, http.StatusBadRequest, "提供商名称不能为空")
		return
	}

	maintenanceLogger.Printf("收到同步 %s 模型列表的请求", provider)

	// 从请求体中获取可选的 baseURL 和 apiKey（如果有）
	var requestBody struct {
		BaseURL string `json:"base_url,omitempty"`
		APIKey  string `json:"api_key,omitempty"`
	}

	// 尝试解析请求体（如果存在）
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			// 如果解析失败，忽略错误，使用默认行为（从数据库获取）
			maintenanceLogger.Printf("无法解析请求体，将使用数据库配置: %v", err)
		}
	}

	// 调用维护服务同步模型
	var err error
	if requestBody.BaseURL != "" {
		// 如果提供了 baseURL，使用提供的配置
		// baseURL[0] = baseURL, baseURL[1] = apiKey (可选)
		if requestBody.APIKey != "" {
			err = maintenanceService.SyncProviderModels(provider, requestBody.BaseURL, requestBody.APIKey)
		} else {
			err = maintenanceService.SyncProviderModels(provider, requestBody.BaseURL)
		}
	} else {
		// 否则从数据库获取配置
		err = maintenanceService.SyncProviderModels(provider)
	}

	if err != nil {
		maintenanceLogger.Printf("❌ 同步 %s 模型失败: %v", provider, err)
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("同步模型失败: %v", err))
		return
	}

	maintenanceLogger.Printf("✅ 成功同步 %s 模型列表", provider)
	respondWithSuccess(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"provider": provider,
		"message":  fmt.Sprintf("成功同步 %s 的模型列表", provider),
	})
}

// handleSyncAllProviderModels 同步所有提供商的模型列表
// 路由: POST /v1/models/sync/all
func handleSyncAllProviderModels(w http.ResponseWriter, r *http.Request) {
	maintenanceLogger.Printf("收到同步所有提供商模型列表的请求")

	// 调用维护服务同步所有模型
	err := maintenanceService.SyncAllProviderModels()

	if err != nil {
		maintenanceLogger.Printf("❌ 同步所有提供商模型失败: %v", err)
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("同步模型失败: %v", err))
		return
	}

	maintenanceLogger.Printf("✅ 成功同步所有提供商的模型列表")
	respondWithSuccess(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "成功同步所有提供商的模型列表",
	})
}

// respondWithError 返回错误响应
func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   true,
		"message": message,
	})
}

// respondWithSuccess 返回成功响应
func respondWithSuccess(w http.ResponseWriter, statusCode int, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
