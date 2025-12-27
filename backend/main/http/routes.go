package http

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

// API路由处理
func setupRoutes() http.Handler {
	router := mux.NewRouter()

	// 注册基本路由
	router.HandleFunc("/", handleRoot).Methods("GET")

	// 注册API路由
	// 统一加密请求接口（除了 getServerPublicKey 和 getNonce，其他都走这个）
	router.Handle("/api/encryptedRequest", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	
	// 普通请求接口（仅用于 getServerPublicKey 和 getNonce）
	router.Handle("/api/plainRequest", rateLimit(http.HandlerFunc(handlePlainRequest), 5, 180)).Methods("POST", "OPTIONS")
	
	// 获取服务器公钥（不加密）
	router.HandleFunc("/api/getServerPublicKey", handlePlainRequest).Methods("POST", "OPTIONS")

	// 获取nonce（不加密，独立处理函数）
	router.HandleFunc("/api/getNonce", handlePlainRequest).Methods("POST", "OPTIONS")

	// 验证码接口
	router.Handle("/api/captcha", rateLimit(http.HandlerFunc(handleEncryptedRequest), 3, 60)).Methods("POST", "OPTIONS")

	// Silicoid 系统接口
	router.Handle("/api/silicoid/chat/completions", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/models", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	// 硅基生命体训练数据管理接口
	router.Handle("/api/silicoid/trainingData/create", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/createBatch", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/get", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/list", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/update", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/updateBatch", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/delete", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/deleteBatch", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/hardDelete", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/silicoid/trainingData/hardDeleteBatch", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// 安全检测指纹库版本管理接口
	router.Handle("/api/securityCheck/fingerprint/version", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/securityCheck/fingerprint/version/check", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	// 安全检测训练数据管理接口
	router.Handle("/api/securityCheck/training/saveDataset", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/securityCheck/training/checkSimilarity", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// AI基础平台接口
	router.Handle("/api/aiBasicPlatform/appVersion/get", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/appVersion/check", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/systemPrompt/list", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/systemPrompt/refresh", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/systemPrompt/refreshAll", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/systemPrompt/getAllRoles", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/translation/get", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/translation/list", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/assetsTokens", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/apiKeyManage/list", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/apiKeyManage/create", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/apiKeyManage/delete", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/aiBasicPlatform/apiKeyManage/updateStatus", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// 统一认证用户状态管理接口
	router.Handle("/api/userStatus/login", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userStatus/logout", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userStatus/deactivation", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userStatus/authToken", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// 统一认证用户信息管理接口
	router.Handle("/api/userInfo/modifyUsername", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userInfo/modifyNickname", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userInfo/modifyMobile", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userInfo/modifyEmail", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// 统一认证用户文件管理接口（除上传下载外）
	router.Handle("/api/userFiles/list", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userFiles/delete", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")
	router.Handle("/api/userFiles/updateAllowedApps", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// 通信系统接口
	router.Handle("/api/communicationSystem/relationshipManagement", rateLimit(http.HandlerFunc(handleEncryptedRequest), 5, 180)).Methods("POST", "OPTIONS")

	// 文件下载接口 - 从 /releases/ 目录提供文件下载
	// 保留独立路由，因为需要返回二进制流
	router.HandleFunc("/api/downloads/{filename}", handleDownload).Methods("GET", "OPTIONS")

	// 用户文件管理接口
	// 文件上传接口 - 保留独立路由，因为需要支持multipart/form-data
	// 地址: http://192.168.100.233/api/userfiles/upload
	// 支持三种上传方式：
	// 1. 加密上传（application/json，包含ciphertext字段）
	// 2. 不加密上传（application/json，直接包含文件数据）
	// 3. 分块上传（multipart/form-data，用于大文件）
	// 注意：也可以通过统一接口使用 type: "userFiles", operation: "upload"（仅支持JSON方式）
	router.HandleFunc("/api/userfiles/upload", handleFileUpload).Methods("POST", "OPTIONS")
	// 文件下载接口 - 保留独立路由，因为需要返回二进制流
	// 注意：也可以通过统一接口使用 type: "userFiles", operation: "download"（返回base64编码）
	router.HandleFunc("/api/userfiles/{file_id}/download", handleFileDownload).Methods("GET", "OPTIONS")
	// 注意：其他用户文件管理接口已统一到 /api/encryptedRequest 或 /api/plainRequest
	// 使用 type: "userFiles", operation: "list|delete|updateAllowedApps"

	// 添加CORS支持
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-App-Id"},
	})

	return corsHandler.Handler(router)
}
