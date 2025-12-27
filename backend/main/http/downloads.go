package http

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// handleDownload 处理文件下载请求
// 从 /releases/ 目录提供文件下载
func handleDownload(w http.ResponseWriter, r *http.Request) {
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	logger.Printf("[%s] 收到文件下载请求", requestID)

	// 处理 CORS 预检请求
	if r.Method == "OPTIONS" {
		buildCorsPreflightResponse(w)
		return
	}

	// 只允许 GET 请求
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 验证 nonce（防重放攻击）
	// 从查询参数获取 nonce，构建 data map
	nonce := r.URL.Query().Get("nonce")
	data := map[string]interface{}{
		"nonce": nonce,
	}
	if err := VerifyNonce(requestID, data); err != nil {
		http.Error(w, "无效请求", http.StatusBadRequest)
		return
	}

	// 从 URL 路径中提取文件名
	// 路径格式: /api/downloads/{filename}
	vars := mux.Vars(r)
	filename, ok := vars["filename"]
	if !ok || filename == "" {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// 防止路径遍历攻击，只允许文件名，不允许路径分隔符
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.Error(w, "Invalid file name", http.StatusBadRequest)
		return
	}

	// 获取当前工作目录（应该已经是项目根目录，由 main.go 的 ensureWorkingDirectory 设置）
	workDir, err := os.Getwd()
	if err != nil {
		logger.Printf("获取工作目录失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// 构建文件完整路径
	// releases 目录应该在项目根目录下
	// 项目结构: digitalsingularity/
	//   - main.go
	//   - go.mod
	//   - releases/
	//   - backend/
	releasesDir := filepath.Join(workDir, "releases")
	filePath := filepath.Join(releasesDir, filename)
	
	// 清理路径，防止路径遍历攻击
	filePath = filepath.Clean(filePath)
	
	// 再次验证路径安全性：确保文件路径在 releases 目录内
	releasesDirAbs, err := filepath.Abs(releasesDir)
	if err != nil {
		logger.Printf("获取 releases 目录绝对路径失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	filePathAbs, err := filepath.Abs(filePath)
	if err != nil {
		logger.Printf("获取文件绝对路径失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	// 确保文件路径在 releases 目录内（防止路径遍历攻击）
	if !strings.HasPrefix(filePathAbs, releasesDirAbs) {
		logger.Printf("路径遍历攻击尝试: %s", filePathAbs)
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// 验证文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Printf("文件不存在: %s", filePath)
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		logger.Printf("检查文件失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// 检查是否是文件（不是目录）
	if fileInfo.IsDir() {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		logger.Printf("打开文件失败: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// 设置响应头
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// 记录下载请求
	logger.Printf("文件下载: %s (大小: %d 字节) 来自 %s", filename, fileInfo.Size(), r.RemoteAddr)

	// 将文件内容写入响应
	_, err = io.Copy(w, file)
	if err != nil {
		logger.Printf("写入响应失败: %v", err)
		return
	}
}

