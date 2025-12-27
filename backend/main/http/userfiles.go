package http

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"digitalsingularity/backend/common/userfiles"
)

var fileService *userfiles.FileService

// 初始化文件服务
func init() {
	fileService = userfiles.NewFileService()
}

// getAppIdFromRequest 从请求中获取应用ID（从请求头或查询参数）
func getAppIdFromRequest(r *http.Request) string {
	// 优先从请求头获取
	appId := r.Header.Get("X-App-Id")
	if appId != "" {
		return appId
	}
	
	// 从查询参数获取
	appId = r.URL.Query().Get("app_id")
	return appId
}

// handleFileUpload 处理文件上传（支持分块上传和加密）
func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	logger.Printf("[%s] 收到文件上传请求", requestID)

	if r.Method == "OPTIONS" {
		buildCorsPreflightResponse(w)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 从请求头或查询参数获取authToken
	authToken := r.Header.Get("Authorization")
	if authToken != "" && strings.HasPrefix(authToken, "Bearer ") {
		authToken = authToken[7:]
	} else {
		authToken = r.URL.Query().Get("authToken")
	}

	if authToken == "" {
		logger.Printf("[%s] 缺少authToken", requestID)
		respondWithError(w, "缺少认证authToken", http.StatusUnauthorized)
		return
	}

	// 验证authToken并获取userid
	valid, payload := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		logger.Printf("[%s] authToken验证失败", requestID)
		respondWithError(w, "无效的authToken", http.StatusUnauthorized)
		return
	}

	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		logger.Printf("[%s] authToken payload格式错误", requestID)
		respondWithError(w, "authToken格式错误", http.StatusUnauthorized)
		return
	}

	userId, ok := payloadMap["userId"].(string)
	if !ok || userId == "" {
		logger.Printf("[%s] 无法从token获取userid", requestID)
		respondWithError(w, "无法获取用户ID", http.StatusUnauthorized)
		return
	}

	logger.Printf("[%s] 用户ID: %s", requestID, userId)

	// 检查Content-Type
	contentType := r.Header.Get("Content-Type")
	
	// 判断是否是JSON请求
	if strings.Contains(contentType, "application/json") {
		// 先读取请求体以检测是否包含ciphertext字段
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Printf("[%s] 读取请求体失败: %v", requestID, err)
			respondWithError(w, "读取请求数据失败", http.StatusBadRequest)
			return
		}

		// 尝试解析为JSON以检测是否包含ciphertext
		var requestData map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &requestData); err != nil {
			logger.Printf("[%s] 解析请求体失败: %v", requestID, err)
			respondWithError(w, "无效的请求数据", http.StatusBadRequest)
			return
		}

		var uploadData map[string]interface{}
		var userPublicKey string
		isEncrypted := false

		// 检查是否包含ciphertext字段（加密请求）
		if ciphertext, ok := requestData["ciphertext"].(string); ok && ciphertext != "" {
			isEncrypted = true
			logger.Printf("[%s] 检测到加密请求", requestID)

			// 使用统一的解密函数
			decryptedData, err := decryptRequestData(ciphertext, requestID)
			if err != nil {
				respondWithError(w, err.Error(), http.StatusBadRequest)
				return
			}

			// 解析解密后的数据
			if err := json.Unmarshal([]byte(decryptedData), &uploadData); err != nil {
				logger.Printf("[%s] 解析解密后的JSON数据失败: %v", requestID, err)
				respondWithError(w, "无效的请求数据", http.StatusBadRequest)
				return
			}

			// 验证 nonce（必需，用于防止重放攻击）
			if err := VerifyNonce(requestID, uploadData); err != nil {
				respondWithError(w, "无效请求", http.StatusBadRequest)
				return
			}

			// 获取用户公钥（优先使用 userPublicKeyHex，向后兼容 userPublicKeyBase64）
			userPublicKey, _ = uploadData["userPublicKeyHex"].(string)
			if userPublicKey == "" {
				userPublicKey, _ = uploadData["userPublicKeyBase64"].(string)
			}
		} else {
			// 不加密请求：直接使用请求数据
			logger.Printf("[%s] 检测到不加密请求", requestID)
			uploadData = requestData
			
			// 验证 nonce（必需，用于防止重放攻击）
			if err := VerifyNonce(requestID, uploadData); err != nil {
				respondWithError(w, "无效请求", http.StatusBadRequest)
				return
			}
		}

		// 处理文件上传
		result := processEncryptedFileUpload(requestID, userId, uploadData)
		
		// 如果是加密请求且有用户公钥，则使用统一的加密函数
		if isEncrypted && userPublicKey != "" {
			encryptedResult, err := encryptResponseData(result, userPublicKey, requestID)
			if err == nil {
				json.NewEncoder(w).Encode(map[string]string{
					"ciphertext": encryptedResult,
				})
				return
			}
			// 加密失败时，继续返回未加密的响应
		}
		
		// 返回不加密的响应
		json.NewEncoder(w).Encode(result)
	} else if strings.Contains(contentType, "multipart/form-data") {
		// 普通multipart上传（用于大文件分块上传）
		processMultipartFileUpload(w, r, requestID, userId)
	} else {
		logger.Printf("[%s] 不支持的Content-Type: %s", requestID, contentType)
		respondWithError(w, "不支持的Content-Type", http.StatusBadRequest)
		return
	}
}

// processEncryptedFileUpload 处理加密的文件上传（小文件，16进制编码）
func processEncryptedFileUpload(requestID, userId string, uploadData map[string]interface{}) map[string]interface{} {
	// 获取文件数据
	fileData, ok := uploadData["file_data"].(string)
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少文件数据",
		}
	}

	originalName, _ := uploadData["file_name"].(string)
	fileType, _ := uploadData["file_type"].(string)
	mimeType, _ := uploadData["mime_type"].(string)
	fileSize, _ := uploadData["file_size"].(float64)
	fileId, _ := uploadData["file_id"].(string)
	fileHash, _ := uploadData["file_hash"].(string)

	// 获取allowed_apps（如果提供）
	var allowedApps interface{} = nil
	if apps, ok := uploadData["allowed_apps"]; ok && apps != nil {
		allowedApps = apps
	}

	// 调用服务层上传文件
	req := &userfiles.UploadFileRequest{
		FileId:      fileId,
		FileHash:    fileHash,
		FileData:    fileData,
		FileName:    originalName,
		FileType:    fileType,
		MimeType:    mimeType,
		FileSize:    int64(fileSize),
		AllowedApps: allowedApps,
	}

	result, err := fileService.UploadFile(userId, req)
	if err != nil {
		logger.Printf("[%s] 文件上传失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	if !result.Success {
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	logger.Printf("[%s] 文件上传成功: %s", requestID, result.FileId)
	return map[string]interface{}{
		"status":  "success",
		"file_id": result.FileId,
		"file_hash": result.FileHash,
		"message": result.Message,
	}
}

// processMultipartFileUpload 处理multipart文件上传（支持分块上传）
func processMultipartFileUpload(w http.ResponseWriter, r *http.Request, requestID, userId string) {
	// 解析multipart form
	err := r.ParseMultipartForm(100 << 20) // 100MB
	if err != nil {
		logger.Printf("[%s] 解析multipart form失败: %v", requestID, err)
		respondWithError(w, "解析表单失败", http.StatusBadRequest)
		return
	}

	// 验证 nonce（必需，用于防止重放攻击）
	// 从 form 中获取 nonce，构建 data map
	nonce := r.FormValue("nonce")
	data := map[string]interface{}{
		"nonce": nonce,
	}
	if err := VerifyNonce(requestID, data); err != nil {
		respondWithError(w, "无效请求", http.StatusBadRequest)
		return
	}

	// 获取分块信息
	chunkIndexStr := r.FormValue("chunk_index")
	chunkTotalStr := r.FormValue("chunk_total")
	fileId := r.FormValue("file_id")
	fileHash := r.FormValue("file_hash")
	originalName := r.FormValue("file_name")
	fileType := r.FormValue("file_type")
	mimeType := r.FormValue("mime_type")
	fileSizeStr := r.FormValue("file_size")

	chunkIndex, _ := strconv.Atoi(chunkIndexStr)
	chunkTotal, _ := strconv.Atoi(chunkTotalStr)
	fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)

	// 获取allowed_apps（如果提供）
	var allowedApps interface{} = nil
	allowedAppsStr := r.FormValue("allowed_apps")
	if allowedAppsStr != "" {
		allowedApps = userfiles.ParseAllowedAppsFromString(allowedAppsStr)
	}

	// 获取文件
	file, _, err := r.FormFile("file")
	if err != nil {
		logger.Printf("[%s] 获取文件失败: %v", requestID, err)
		respondWithError(w, "获取文件失败", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 调用服务层上传分块
	req := &userfiles.ChunkUploadRequest{
		ChunkIndex:  chunkIndex,
		ChunkTotal:  chunkTotal,
		FileId:      fileId,
		FileHash:    fileHash,
		FileName:    originalName,
		FileType:    fileType,
		MimeType:    mimeType,
		FileSize:    fileSize,
		AllowedApps: allowedApps,
		ChunkData:   file,
	}

	result, err := fileService.UploadChunk(userId, req)
	if err != nil {
		logger.Printf("[%s] 分块上传失败: %v", requestID, err)
		respondWithError(w, result.Message, http.StatusInternalServerError)
		return
	}

	if !result.Success {
		respondWithError(w, result.Message, http.StatusInternalServerError)
		return
	}

	// 返回结果
	response := map[string]interface{}{
		"status":        result.Status,
		"file_id":       result.FileId,
		"file_hash":     result.FileHash,
		"chunk_index":   result.ChunkIndex,
		"chunk_total":   result.ChunkTotal,
		"chunk_uploaded": result.ChunkUploaded,
	}
	if result.Message != "" {
		response["message"] = result.Message
	}

	json.NewEncoder(w).Encode(response)
}

// handleFileDownload 处理文件下载
func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	requestID := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	logger.Printf("[%s] 收到文件下载请求", requestID)

	if r.Method == "OPTIONS" {
		buildCorsPreflightResponse(w)
		return
	}

	// 从请求头或查询参数获取authToken
	authToken := r.Header.Get("Authorization")
	if authToken != "" && strings.HasPrefix(authToken, "Bearer ") {
		authToken = authToken[7:]
	} else {
		authToken = r.URL.Query().Get("authToken")
	}

	if authToken == "" {
		respondWithError(w, "缺少认证authToken", http.StatusUnauthorized)
		return
	}

	// 验证authToken并获取userid
	valid, payload := authTokenService.VerifyAuthToken(authToken)
	if !valid {
		respondWithError(w, "无效的authToken", http.StatusUnauthorized)
		return
	}

	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		respondWithError(w, "authToken格式错误", http.StatusUnauthorized)
		return
	}

	userId, ok := payloadMap["userId"].(string)
	if !ok || userId == "" {
		respondWithError(w, "无法获取用户ID", http.StatusUnauthorized)
		return
	}

	// 获取file_id
	vars := mux.Vars(r)
	fileId := vars["file_id"]
	if fileId == "" {
		respondWithError(w, "缺少文件ID", http.StatusBadRequest)
		return
	}

	// 获取应用ID
	appId := getAppIdFromRequest(r)

	// 调用服务层下载文件
	req := &userfiles.DownloadFileRequest{
		FileId: fileId,
		AppId:  appId,
	}

	result, err := fileService.DownloadFile(userId, req)
	if err != nil {
		logger.Printf("[%s] 下载文件失败: %v", requestID, err)
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "无权访问") {
			statusCode = http.StatusForbidden
		} else if strings.Contains(err.Error(), "不存在") {
			statusCode = http.StatusNotFound
		} else if strings.Contains(err.Error(), "尚未上传完成") {
			statusCode = http.StatusBadRequest
		}
		respondWithError(w, result.Message, statusCode)
		return
	}

	if !result.Success {
		respondWithError(w, result.Message, http.StatusInternalServerError)
		return
	}

	// 打开文件
	file, err := os.Open(result.FilePath)
	if err != nil {
		logger.Printf("[%s] 打开文件失败: %v", requestID, err)
		respondWithError(w, "文件不存在", http.StatusNotFound)
		return
	}
	defer file.Close()

	// 设置响应头
	w.Header().Set("Content-Type", result.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", result.OriginalName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", result.FileSize))
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 发送文件
	io.Copy(w, file)
	logger.Printf("[%s] 文件下载成功: %s", requestID, fileId)
}

// ========== 业务逻辑函数（供统一接口使用） ==========

// handleFileUploadRequest 处理文件上传请求（业务逻辑函数）
func handleFileUploadRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到文件上传请求（业务逻辑）", requestID)

	// 获取用户ID
	userId, ok := data["user_id"].(string)
	if !ok || userId == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 处理文件上传（复用现有逻辑）
	return processEncryptedFileUpload(requestID, userId, data)
}

// handleFileListRequest 处理获取文件列表请求（业务逻辑函数）
func handleFileListRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到文件列表请求（业务逻辑）", requestID)

	// 获取用户ID
	userId, ok := data["user_id"].(string)
	if !ok || userId == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 获取应用ID（可选）
	appId, _ := data["app_id"].(string)

	// 调用服务层获取文件列表
	req := &userfiles.ListFilesRequest{
		AppId: appId,
	}

	result, err := fileService.ListFiles(userId, req)
	if err != nil {
		logger.Printf("[%s] 获取文件列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "获取文件列表失败",
		}
	}

	if !result.Success {
		return map[string]interface{}{
			"status":  "fail",
			"message": "获取文件列表失败",
		}
	}

	logger.Printf("[%s] 获取到 %d 个文件", requestID, result.Count)
	return map[string]interface{}{
		"status": "success",
		"files":  result.Files,
		"count":  result.Count,
	}
}

// handleFileDeleteRequest 处理文件删除请求（业务逻辑函数）
func handleFileDeleteRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到文件删除请求（业务逻辑）", requestID)

	// 获取用户ID
	userId, ok := data["user_id"].(string)
	if !ok || userId == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 获取file_id
	fileId, ok := data["file_id"].(string)
	if !ok || fileId == "" {
		logger.Printf("[%s] 缺少文件ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少文件ID",
		}
	}

	// 调用服务层删除文件
	req := &userfiles.DeleteFileRequest{
		FileId: fileId,
	}

	result, err := fileService.DeleteFile(userId, req)
	if err != nil {
		logger.Printf("[%s] 删除文件失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	if !result.Success {
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	logger.Printf("[%s] 文件删除成功: %s", requestID, fileId)
	return map[string]interface{}{
		"status":  "success",
		"message": result.Message,
	}
}

// handleFileDownloadRequest 处理文件下载请求（业务逻辑函数）
// 注意：文件下载返回的是文件数据（base64编码），而不是直接流式传输
func handleFileDownloadRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到文件下载请求（业务逻辑）", requestID)

	// 获取用户ID
	userId, ok := data["user_id"].(string)
	if !ok || userId == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 获取file_id
	fileId, ok := data["file_id"].(string)
	if !ok || fileId == "" {
		logger.Printf("[%s] 缺少文件ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少文件ID",
		}
	}

	// 获取应用ID（可选）
	appId, _ := data["app_id"].(string)

	// 调用服务层下载文件
	req := &userfiles.DownloadFileRequest{
		FileId: fileId,
		AppId:  appId,
	}

	result, err := fileService.DownloadFile(userId, req)
	if err != nil {
		logger.Printf("[%s] 下载文件失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	if !result.Success {
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	// 读取文件内容并转换为base64
	fileContent, err := os.ReadFile(result.FilePath)
	if err != nil {
		logger.Printf("[%s] 读取文件失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": "读取文件失败",
		}
	}

	// 将文件内容编码为base64
	fileBase64 := base64.StdEncoding.EncodeToString(fileContent)

	logger.Printf("[%s] 文件下载成功: %s", requestID, fileId)
	return map[string]interface{}{
		"status":        "success",
		"file_id":       fileId,
		"file_name":     result.OriginalName,
		"mime_type":     result.MimeType,
		"file_size":     result.FileSize,
		"file_data":     fileBase64,
		"file_data_encoding": "base64",
	}
}

// handleFileUpdateAllowedAppsRequest 处理更新文件允许访问应用列表的请求（业务逻辑函数）
func handleFileUpdateAllowedAppsRequest(requestID string, data map[string]interface{}) map[string]interface{} {
	logger.Printf("[%s] 收到更新文件允许访问应用列表请求（业务逻辑）", requestID)

	// 获取用户ID
	userId, ok := data["user_id"].(string)
	if !ok || userId == "" {
		logger.Printf("[%s] 缺少用户ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少用户ID",
		}
	}

	// 获取file_id
	fileId, ok := data["file_id"].(string)
	if !ok || fileId == "" {
		logger.Printf("[%s] 缺少文件ID", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少文件ID",
		}
	}

	// 获取allowed_apps
	allowedApps, ok := data["allowed_apps"]
	if !ok {
		logger.Printf("[%s] 缺少allowed_apps参数", requestID)
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少allowed_apps参数",
		}
	}

	// 调用服务层更新允许访问应用列表
	req := &userfiles.UpdateAllowedAppsRequest{
		FileId:      fileId,
		AllowedApps: allowedApps,
	}

	result, err := fileService.UpdateAllowedApps(userId, req)
	if err != nil {
		logger.Printf("[%s] 更新文件允许访问应用列表失败: %v", requestID, err)
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	if !result.Success {
		return map[string]interface{}{
			"status":  "fail",
			"message": result.Message,
		}
	}

	logger.Printf("[%s] 更新文件允许访问应用列表成功: %s", requestID, fileId)
	return map[string]interface{}{
		"status":  "success",
		"message": result.Message,
	}
}


