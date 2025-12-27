package http

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

// SilicoID API路由处理
func setupRoutes() http.Handler {
	router := mux.NewRouter()
	
	// 注册基本路由
	router.HandleFunc("/", handleRoot).Methods("GET")
	
	// 注册OpenAI兼容接口
	router.HandleFunc("/v1/chat/completions", silicoidChatCompletions).Methods("POST", "OPTIONS")
	router.HandleFunc("/v1/models", silicoidModels).Methods("GET", "OPTIONS")
	
	// 注册模型维护接口
	// 注意：必须先注册更具体的路由（/all），再注册带参数的路由（/{provider}）
	router.HandleFunc("/v1/models/sync/all", handleSyncAllProviderModels).Methods("POST", "OPTIONS")
	router.HandleFunc("/v1/models/sync/{provider}", handleSyncProviderModels).Methods("POST", "OPTIONS")
	
	// 添加CORS支持
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})
	
	return corsHandler.Handler(router)
}