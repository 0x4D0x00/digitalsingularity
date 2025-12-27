package http

// HTTP MCP服务器
import (
	"fmt"
	"log"
	"net/http"
)

// 全局日志器
var logger *log.Logger

// 初始化日志器
func init() {
	logger = log.New(log.Writer(), "[MCP-HTTP] ", log.LstdFlags)
	logger.Printf("✅ MCP HTTP服务初始化成功")
}

// HandleConnection 启动MCP HTTP服务
func HandleConnection(host string, port int, debug bool) {
	// 设置路由
	handler := setupRoutes()

	// 创建服务地址
	addr := fmt.Sprintf("%s:%d", host, port)

	// 创建服务器
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// 打印启动信息
	logger.Printf("MCP HTTP服务器启动于 %s", addr)
	if debug {
		logger.Printf("调试模式已启用")
	}

	// 启动服务器
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("MCP HTTP服务器启动失败: %v", err)
	}
}