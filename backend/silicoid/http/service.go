package http

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"digitalsingularity/backend/silicoid/interceptor"
)

// 全局日志器
var logger *log.Logger

// 拦截器实例
var interceptorService *interceptor.SilicoIDInterceptor
var interceptorOnce sync.Once

// 初始化日志器
func init() {
	// 初始化日志器
	logger = log.New(log.Writer(), "[SilicoID] ", log.LstdFlags)
	logger.Printf("✅ SilicoID HTTP服务初始化成功")
}

// SilicoID主页处理函数
func handleRoot(w http.ResponseWriter, r *http.Request) {
	logger.Printf("接收到SilicoID请求: %s %s", r.Method, r.URL.Path)
	fmt.Fprintf(w, "SilicoID HTTP 服务")
}

// 创建一个辅助函数来包装http.ResponseWriter和http.Request为gin.Context
func createGinContext(w http.ResponseWriter, r *http.Request) *gin.Context {
	// 创建一个新的上下文
	c, _ := gin.CreateTestContext(w)
	// 设置请求和响应
	c.Request = r
	c.Writer = &responseWriterWrapper{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
	return c
}

// 响应写入器包装器，实现gin.ResponseWriter接口
type responseWriterWrapper struct {
	http.ResponseWriter
	StatusCode int
	size       int
}

func (w *responseWriterWrapper) Status() int {
	return w.StatusCode
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) WriteHeaderNow() {
	if w.StatusCode == 0 {
		w.StatusCode = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.StatusCode)
}

func (w *responseWriterWrapper) Write(data []byte) (int, error) {
	size, err := w.ResponseWriter.Write(data)
	w.size += size
	return size, err
}

func (w *responseWriterWrapper) WriteString(s string) (int, error) {
	size, err := w.ResponseWriter.Write([]byte(s))
	w.size += size
	return size, err
}

func (w *responseWriterWrapper) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriterWrapper) CloseNotify() <-chan bool {
	if cn, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return make(<-chan bool)
}

func (w *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker接口未实现")
}

func (w *responseWriterWrapper) Pusher() http.Pusher {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher
	}
	return nil
}

func (w *responseWriterWrapper) Size() int {
	return w.size
}

func (w *responseWriterWrapper) Written() bool {
	return w.size > 0
}

// SilicoidHandleConnection 启动SilicoID HTTP服务
func SilicoidHandleConnection(host string, port int, debug bool) {
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
	logger.Printf("SilicoID HTTP服务器启动于 %s", addr)
	if debug {
		logger.Printf("调试模式已启用")
	}
	
	// 启动服务器
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("SilicoID HTTP服务器启动失败: %v", err)
	}
} 