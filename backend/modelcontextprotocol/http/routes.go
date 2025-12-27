package http

// MCP HTTP路由配置
import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"digitalsingularity/backend/modelcontextprotocol/server"
)

// MCP根路由处理
func handleMCPRoot(w http.ResponseWriter, r *http.Request) {
	log.Printf("接收到MCP请求: %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "Model Context Protocol HTTP Server", "version": "1.0.0"}`)
}

// 设置路由
func setupRoutes() http.Handler {
	router := mux.NewRouter()

	// MCP路由
	router.HandleFunc("/mcp", handleMCPRoot).Methods("GET", "POST", "OPTIONS")
	router.HandleFunc("/mcp/current-time", server.CurrentTime).Methods("GET", "OPTIONS")
	router.HandleFunc("/mcp/current-weather", server.CurrentWeather).Methods("GET", "OPTIONS")
	router.HandleFunc("/mcp/storagebox-data-reading", server.StorageboxDataReading).Methods("GET", "OPTIONS")
	router.HandleFunc("/mcp/storagebox-ip-storage", server.StorageboxIPStorage).Methods("POST", "OPTIONS")

	// 添加CORS支持
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-MCP-Version"},
	})

	return corsHandler.Handler(router)
}
