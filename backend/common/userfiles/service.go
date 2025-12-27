package userfiles

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	userfilesdatabase "digitalsingularity/backend/common/userfiles/database"

)

// FileService 文件业务逻辑服务
type FileService struct {
	dbService *userfilesdatabase.UserFileService
}

// NewFileService 创建文件服务实例
func NewFileService() *FileService {
	return &FileService{
		dbService: userfilesdatabase.NewUserFileService(),
	}
}

// UploadFileRequest 文件上传请求
type UploadFileRequest struct {
	FileId       string      // 文件ID（可选，如果提供则使用，否则自动生成）
	FileHash     string      // 文件MD5哈希值（可选，用于去重）
	FileData     string      // base64 或 16进制编码的文件数据（自动检测）
	FileName     string      // 原始文件名
	FileType     string      // 文件类型
	MimeType     string      // MIME类型
	FileSize     int64       // 文件大小
	AllowedApps  interface{} // 允许访问的应用ID列表
}

// UploadFileResult 文件上传结果
type UploadFileResult struct {
	Success bool
	FileId  string
	FileHash string
	Message string
}

// UploadFile 上传文件（加密上传，小文件）
func (s *FileService) UploadFile(userId string, req *UploadFileRequest) (*UploadFileResult, error) {
	// 解码文件数据（自动检测 base64 或 16 进制）
	var fileBytes []byte
	var err error
	
	// 先尝试 base64 解码（更常用）
	fileBytes, err = base64.StdEncoding.DecodeString(req.FileData)
	if err != nil {
		// 如果 base64 解码失败，尝试 16 进制解码（向后兼容）
		fileBytes, err = hex.DecodeString(req.FileData)
		if err != nil {
			log.Printf("文件数据解码失败（base64 和 16 进制都失败）: %v", err)
			return &UploadFileResult{
				Success: false,
				Message: "文件数据格式错误",
			}, err
		}
	}

	// 使用实际文件大小
	actualFileSize := int64(len(fileBytes))
	if req.FileSize > 0 {
		actualFileSize = req.FileSize
	}

	// 计算文件MD5（如果未提供）
	fileHash := req.FileHash
	if fileHash == "" {
		hash := md5.Sum(fileBytes)
		fileHash = fmt.Sprintf("%x", hash)
	}

	// 如果提供了MD5，检查是否已存在相同文件
	if fileHash != "" {
		existingFile, err := s.dbService.GetFileByHash(userId, fileHash)
		if err == nil && existingFile != nil {
			// 文件已存在，返回已存在的文件ID
			existingFileId, ok := existingFile["file_id"].(string)
			if ok {
				log.Printf("文件已存在（MD5: %s），返回已存在的文件ID: %s", fileHash, existingFileId)
				return &UploadFileResult{
					Success: true,
					FileId:  existingFileId,
					FileHash: fileHash,
					Message: "文件已存在，无需重复上传",
				}, nil
			}
		}
	}

	// 确定文件ID
	fileId := req.FileId
	if fileId == "" {
		fileId = uuid.New().String()
	}

	// 获取基础目录（优先使用挂载/本地基础目录，其次工作目录）
	baseDir, err := getBaseDir()
	if err != nil {
		log.Printf("获取基础目录失败: %v", err)
		return &UploadFileResult{
			Success: false,
			Message: "服务器错误",
		}, err
	}

	// 设置默认值
	originalName := req.FileName
	if originalName == "" {
		originalName = "unknown"
	}
	fileType := req.FileType
	if fileType == "" {
		fileType = "other"
	}

	// 从原始文件名中提取扩展名
	ext := filepath.Ext(originalName)
	// 使用 userId+fileId+扩展名 作为文件名，保留原始文件格式
	fileName := fmt.Sprintf("%s_%s%s", userId, fileId, ext)
	filePath := filepath.Join(baseDir, "userfiles", fileName)
	
	// 确保 userfiles 目录存在
	userFilesDir := filepath.Join(baseDir, "userfiles")
	if err := os.MkdirAll(userFilesDir, 0755); err != nil {
		log.Printf("创建 userfiles 目录失败: %v", err)
		return &UploadFileResult{
			Success: false,
			Message: "服务器错误",
		}, err
	}

	// 保存文件（完整保存，不经过格式转换）
	if err := os.WriteFile(filePath, fileBytes, 0644); err != nil {
		log.Printf("保存文件失败: %v", err)
		return &UploadFileResult{
			Success: false,
			Message: "保存文件失败",
		}, err
	}

	// 创建文件记录（相对路径使用 userId+fileId+扩展名）
	relativePath := filepath.Join("userfiles", fileName)
	fileRecord := map[string]interface{}{
		"user_id":       userId,
		"file_id":       fileId,
		"original_name": originalName,
		"file_type":     fileType,
		"mime_type":     req.MimeType,
		"file_size":     actualFileSize,
		"file_path":     relativePath,
		"file_hash":     fileHash,
		"upload_status": "completed",
	}

	// 如果提供了allowed_apps，添加到记录中
	if req.AllowedApps != nil {
		fileRecord["allowed_apps"] = req.AllowedApps
	}

	_, err = s.dbService.CreateFileRecord(fileRecord)
	if err != nil {
		log.Printf("创建文件记录失败: %v", err)
		// 删除已保存的文件
		os.Remove(filePath)
		return &UploadFileResult{
			Success: false,
			Message: "创建文件记录失败",
		}, err
	}

	// 远程推送已移除

	return &UploadFileResult{
		Success: true,
		FileId:  fileId,
		FileHash: fileHash,
		Message: "文件上传成功",
	}, nil
}

// ChunkUploadRequest 分块上传请求
type ChunkUploadRequest struct {
	ChunkIndex   int         // 当前块索引
	ChunkTotal   int         // 总块数
	FileId       string      // 文件ID（如果为空则自动生成）
	FileHash     string      // 文件MD5哈希值（可选，用于去重）
	FileName     string      // 原始文件名
	FileType     string      // 文件类型
	MimeType     string      // MIME类型
	FileSize     int64       // 文件大小
	AllowedApps  interface{} // 允许访问的应用ID列表
	ChunkData    io.Reader   // 分块数据
}

// ChunkUploadResult 分块上传结果
type ChunkUploadResult struct {
	Success      bool
	FileId       string
	FileHash     string
	Status       string // "uploading" 或 "completed"
	ChunkIndex   int
	ChunkTotal   int
	ChunkUploaded int
	Message      string
}

// UploadChunk 上传文件分块
func (s *FileService) UploadChunk(userId string, req *ChunkUploadRequest) (*ChunkUploadResult, error) {
	// 如果是第一块，创建文件记录
	if req.ChunkIndex == 0 {
		if req.FileId == "" {
			req.FileId = uuid.New().String()
		}

		// 获取基础目录（优先使用挂载/本地基础目录，其次工作目录）
		_, err := getBaseDir()
		if err != nil {
			log.Printf("获取基础目录失败: %v", err)
			return &ChunkUploadResult{
				Success: false,
				Message: "服务器错误",
			}, err
		}

		// 如果提供了FileHash，检查是否已存在相同文件
		if req.FileHash != "" {
			existingFile, err := s.dbService.GetFileByHash(userId, req.FileHash)
			if err == nil && existingFile != nil {
				// 文件已存在，返回已存在的文件ID
				existingFileId, ok := existingFile["file_id"].(string)
				if ok {
					log.Printf("文件已存在（MD5: %s），返回已存在的文件ID: %s", req.FileHash, existingFileId)
					return &ChunkUploadResult{
						Success: true,
						FileId:  existingFileId,
						FileHash: req.FileHash,
						Status:  "completed",
						Message: "文件已存在，无需重复上传",
					}, nil
				}
			}
		}

		// 确定文件ID
		if req.FileId == "" {
			req.FileId = uuid.New().String()
		}

		// 从原始文件名中提取扩展名
		ext := filepath.Ext(req.FileName)
		// 创建文件记录（使用 userId+fileId+扩展名 作为文件名，保留原始文件格式）
		fileName := fmt.Sprintf("%s_%s%s", userId, req.FileId, ext)
		relativePath := filepath.Join("userfiles", fileName)
		fileRecord := map[string]interface{}{
			"user_id":       userId,
			"file_id":       req.FileId,
			"original_name": req.FileName,
			"file_type":     req.FileType,
			"mime_type":     req.MimeType,
			"file_size":     req.FileSize,
			"file_path":     relativePath,
			"file_hash":     req.FileHash, // 如果提供了MD5，先保存
			"upload_status": "uploading",
			"chunk_total":   req.ChunkTotal,
			"chunk_uploaded": 0,
		}

		// 如果提供了allowed_apps，添加到记录中
		if req.AllowedApps != nil {
			fileRecord["allowed_apps"] = req.AllowedApps
		}

		_, err = s.dbService.CreateFileRecord(fileRecord)
		if err != nil {
			log.Printf("创建文件记录失败: %v", err)
			return &ChunkUploadResult{
				Success: false,
				Message: "创建文件记录失败",
			}, err
		}
	}

	// 获取基础目录（优先使用挂载/本地基础目录，其次工作目录）
	baseDir, err := getBaseDir()
	if err != nil {
		log.Printf("获取基础目录失败: %v", err)
		return &ChunkUploadResult{
			Success: false,
			Message: "服务器错误",
		}, err
	}
	
	// 确保 userfiles 目录存在
	userFilesDir := filepath.Join(baseDir, "userfiles")
	if err := os.MkdirAll(userFilesDir, 0755); err != nil {
		log.Printf("创建 userfiles 目录失败: %v", err)
		return &ChunkUploadResult{
			Success: false,
			Message: "服务器错误",
		}, err
	}
	
	// 从原始文件名中提取扩展名
	ext := filepath.Ext(req.FileName)
	// 使用 userId+fileId+扩展名 作为文件名前缀（保留原始文件格式）
	fileName := fmt.Sprintf("%s_%s%s", userId, req.FileId, ext)
	chunkPath := filepath.Join(userFilesDir, fmt.Sprintf("%s.chunk.%d", fileName, req.ChunkIndex))

	// 保存分块
	dst, err := os.Create(chunkPath)
	if err != nil {
		log.Printf("创建分块文件失败: %v", err)
		return &ChunkUploadResult{
			Success: false,
			Message: "保存文件失败",
		}, err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, req.ChunkData); err != nil {
		log.Printf("写入分块文件失败: %v", err)
		return &ChunkUploadResult{
			Success: false,
			Message: "保存文件失败",
		}, err
	}

	// 更新上传进度
	s.dbService.UpdateFileStatus(req.FileId, "uploading", req.ChunkIndex+1)

	// 如果是最后一块，合并所有分块
	if req.ChunkIndex == req.ChunkTotal-1 {
		// 从原始文件名中提取扩展名
		ext := filepath.Ext(req.FileName)
		// 使用 userId+fileId+扩展名 作为最终文件名（保留原始文件格式）
		fileName := fmt.Sprintf("%s_%s%s", userId, req.FileId, ext)
		finalPath := filepath.Join(userFilesDir, fileName)
		finalFile, err := os.Create(finalPath)
		if err != nil {
			log.Printf("创建最终文件失败: %v", err)
			return &ChunkUploadResult{
				Success: false,
				Message: "合并文件失败",
			}, err
		}
		defer finalFile.Close()

		// 合并所有分块并计算MD5
		hash := md5.New()
		for i := 0; i < req.ChunkTotal; i++ {
			chunkPath := filepath.Join(userFilesDir, fmt.Sprintf("%s.chunk.%d", fileName, i))
			chunkFile, err := os.Open(chunkPath)
			if err != nil {
				log.Printf("打开分块文件失败: %v", err)
				return &ChunkUploadResult{
					Success: false,
					Message: "合并文件失败",
				}, err
			}

			// 同时写入最终文件和MD5计算器
			multiWriter := io.MultiWriter(finalFile, hash)
			io.Copy(multiWriter, chunkFile)
			chunkFile.Close()
			os.Remove(chunkPath) // 删除分块文件
		}
		finalFile.Close()

		// 计算文件MD5
		fileHash := fmt.Sprintf("%x", hash.Sum(nil))
		
		// 如果之前没有提供MD5，或者提供的MD5与计算的不一致，使用计算的MD5
		if req.FileHash == "" || req.FileHash != fileHash {
			// 检查是否已存在相同文件
			existingFile, err := s.dbService.GetFileByHash(userId, fileHash)
			if err == nil && existingFile != nil {
				// 文件已存在，删除刚上传的文件，返回已存在的文件ID
				existingFileId, ok := existingFile["file_id"].(string)
				if ok {
					os.Remove(finalPath) // 删除刚上传的文件
					s.dbService.DeleteFile(req.FileId, userId) // 删除文件记录
					log.Printf("文件已存在（MD5: %s），返回已存在的文件ID: %s", fileHash, existingFileId)
					return &ChunkUploadResult{
						Success: true,
						FileId:  existingFileId,
						Status:  "completed",
						Message: "文件已存在，无需重复上传",
					}, nil
				}
			}
		}

		// 更新文件状态为完成，并更新MD5
		s.dbService.UpdateFileStatusWithHash(req.FileId, "completed", req.ChunkTotal, fileHash)

		// 远程推送已移除

		return &ChunkUploadResult{
			Success: true,
			FileId:  req.FileId,
			FileHash: fileHash,
			Status:  "completed",
			Message: "文件上传成功",
		}, nil
	}

	// 返回进度
	return &ChunkUploadResult{
		Success:       true,
		FileId:        req.FileId,
		Status:        "uploading",
		ChunkIndex:    req.ChunkIndex,
		ChunkTotal:    req.ChunkTotal,
		ChunkUploaded: req.ChunkIndex + 1,
	}, nil
}

// DownloadFileRequest 文件下载请求
type DownloadFileRequest struct {
	FileId string // 文件ID
	AppId  string // 应用ID（用于权限验证，如果为空则不验证）
}

// DownloadFileResult 文件下载结果
type DownloadFileResult struct {
	Success      bool
	FilePath     string
	OriginalName string
	MimeType     string
	FileSize     int64
	Message      string
}

// 获取本地基础目录（优先环境变量），若不可用则返回空
func getLocalBaseDir() (string, error) {
	// 优先读取环境变量
	if env := strings.TrimSpace(os.Getenv("USERFILES_LOCAL_BASEDIR")); env != "" {
		if fi, err := os.Stat(env); err == nil && fi.IsDir() {
			return env, nil
		}
	}
	// 不再强制固定路径，交由调用方决定是否回退
	return "", fmt.Errorf("local base dir not configured or unavailable")
}

// 获取工作目录作为回退基础目录
func getWorkingBaseDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd, nil
}

// getBaseDir 获取基础目录（优先环境变量，其次工作目录，最后临时目录）
func getBaseDir() (string, error) {
	// 1) 优先环境变量指定目录
	if base, err := getLocalBaseDir(); err == nil && base != "" {
		return base, nil
	}
	// 2) 回退工作目录
	if wd, err := getWorkingBaseDir(); err == nil && wd != "" {
		return wd, nil
	}
	// 3) 最后回退临时目录
	fallbackBase := filepath.Join(os.TempDir(), "digitalsingularity")
	return fallbackBase, nil
}

// 根据相对路径解析本地实际路径，按多种候选基础目录依次尝试
func resolveLocalFilePath(relativePath string) (string, bool) {
	// 1) 尝试环境变量指定的基础目录
	if base, err := getLocalBaseDir(); err == nil && base != "" {
		p := filepath.Join(base, relativePath)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	// 2) 回退到工作目录
	if wd, err := getWorkingBaseDir(); err == nil && wd != "" {
		p := filepath.Join(wd, relativePath)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// resolvePreferredLocalPath 优先返回挂载/本地基础目录下的路径，其次工作目录
func resolvePreferredLocalPath(relativePath string) (string, bool) {
	baseDir, err := getBaseDir()
	if err != nil {
		return "", false
	}
	p := filepath.Join(baseDir, relativePath)
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

// DownloadFile 下载文件
func (s *FileService) DownloadFile(userId string, req *DownloadFileRequest) (*DownloadFileResult, error) {
	// 验证文件所有权
	owned, err := s.dbService.VerifyFileOwnership(req.FileId, userId)
	if err != nil {
		return &DownloadFileResult{
			Success: false,
			Message: "验证文件所有权失败",
		}, err
	}

	// 获取文件信息
	// 如果是文件所有者，不进行应用权限验证（传入空字符串）
	// 如果不是文件所有者，需要检查应用权限
	var fileInfo map[string]interface{}
	if owned {
		fileInfo, err = s.dbService.GetFileByFileId(req.FileId, "")
	} else {
		// 非文件所有者，需要检查应用权限
		if req.AppId == "" {
			return &DownloadFileResult{
				Success: false,
				Message: "无权访问此文件",
			}, fmt.Errorf("无权访问此文件")
		}
		fileInfo, err = s.dbService.GetFileByFileId(req.FileId, req.AppId)
	}

	if err != nil {
		if strings.Contains(err.Error(), "无权访问") {
			return &DownloadFileResult{
				Success: false,
				Message: "无权访问此文件",
			}, err
		}
		return &DownloadFileResult{
			Success: false,
			Message: "文件不存在",
		}, err
	}

	// 检查文件状态
	if fileInfo["upload_status"].(string) != "completed" {
		return &DownloadFileResult{
			Success: false,
			Message: "文件尚未上传完成",
		}, fmt.Errorf("文件尚未上传完成")
	}

	// 获取相对路径
	relativePath := fileInfo["file_path"].(string)

	// 仅使用本地固定基础目录
	var filePath string
	if p, ok := resolvePreferredLocalPath(relativePath); ok {
		filePath = p
	} else {
		log.Printf("[UserFiles] 本地未找到文件: relative=%s", relativePath)
		return &DownloadFileResult{
			Success: false,
			Message: fmt.Sprintf("文件不存在: relative=%s", relativePath),
		}, fmt.Errorf("文件不存在")
	}

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("[UserFiles] 初始路径不存在: %s", filePath)
		// 直接失败（不做向后兼容的多候选与工作目录回退）
		return &DownloadFileResult{
			Success: false,
			Message: fmt.Sprintf("文件不存在: relative=%s", relativePath),
		}, err
	}

	// 获取文件信息
	fileStat, err := os.Stat(filePath)
	if err != nil {
		return &DownloadFileResult{
			Success: false,
			Message: "服务器错误",
		}, err
	}

	originalName := fileInfo["original_name"].(string)
	mimeType := ""
	if mt, ok := fileInfo["mime_type"].(string); ok {
		mimeType = mt
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return &DownloadFileResult{
		Success:      true,
		FilePath:     filePath,
		OriginalName: originalName,
		MimeType:     mimeType,
		FileSize:     fileStat.Size(),
	}, nil
}

// 远程相关功能与配置读取均已移除，系统仅使用固定本地目录

// tryPushToRemoteIfConfigured 将本地文件推送到远程 SFTP 的对应路径（baseDir + relativePath）
// 最佳努力：若配置缺失或失败，不返回致命错误
// 远程推送功能已移除

// ensureRemoteDirs 递归创建远程目录（等价于 mkdir -p）
// 远程目录创建辅助已移除

// DeleteFileRequest 文件删除请求
type DeleteFileRequest struct {
	FileId string // 文件ID
}

// DeleteFileResult 文件删除结果
type DeleteFileResult struct {
	Success bool
	Message string
}

// DeleteFile 删除文件
func (s *FileService) DeleteFile(userId string, req *DeleteFileRequest) (*DeleteFileResult, error) {
	// 验证文件所有权
	owned, err := s.dbService.VerifyFileOwnership(req.FileId, userId)
	if err != nil {
		return &DeleteFileResult{
			Success: false,
			Message: "验证文件所有权失败",
		}, err
	}

	if !owned {
		return &DeleteFileResult{
			Success: false,
			Message: "无权删除此文件",
		}, fmt.Errorf("无权删除此文件")
	}

	// 获取文件信息（文件所有者可以访问，不进行应用权限验证）
	fileInfo, err := s.dbService.GetFileByFileId(req.FileId, "")
	if err != nil {
		return &DeleteFileResult{
			Success: false,
			Message: "文件不存在",
		}, err
	}

	// 删除物理文件
	relativePath := fileInfo["file_path"].(string)
	if p, ok := resolvePreferredLocalPath(relativePath); ok {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log.Printf("删除物理文件失败 path=%s err=%v", p, err)
			// 继续执行，即使物理文件删除失败也标记为已删除
		}
	} else {
		log.Printf("删除物理文件时未找到文件: %s", relativePath)
	}

	// 软删除文件记录
	if err := s.dbService.DeleteFile(req.FileId, userId); err != nil {
		return &DeleteFileResult{
			Success: false,
			Message: "删除文件失败",
		}, err
	}

	return &DeleteFileResult{
		Success: true,
		Message: "文件删除成功",
	}, nil
}

// ListFilesRequest 文件列表请求
type ListFilesRequest struct {
	AppId string // 应用ID（用于过滤文件，如果为空则返回所有文件）
}

// ListFilesResult 文件列表结果
type ListFilesResult struct {
	Success bool
	Files   []map[string]interface{}
	Count   int
}

// ListFiles 获取文件列表
func (s *FileService) ListFiles(userId string, req *ListFilesRequest) (*ListFilesResult, error) {
	files, err := s.dbService.GetUserFiles(userId, req.AppId)
	if err != nil {
		return &ListFilesResult{
			Success: false,
		}, err
	}

	return &ListFilesResult{
		Success: true,
		Files:   files,
		Count:   len(files),
	}, nil
}

// UpdateAllowedAppsRequest 更新允许访问应用列表请求
type UpdateAllowedAppsRequest struct {
	FileId      string      // 文件ID
	AllowedApps interface{} // 允许访问的应用ID列表
}

// UpdateAllowedAppsResult 更新允许访问应用列表结果
type UpdateAllowedAppsResult struct {
	Success bool
	Message string
}

// UpdateAllowedApps 更新文件的允许访问应用列表
func (s *FileService) UpdateAllowedApps(userId string, req *UpdateAllowedAppsRequest) (*UpdateAllowedAppsResult, error) {
	// 验证文件所有权
	owned, err := s.dbService.VerifyFileOwnership(req.FileId, userId)
	if err != nil {
		return &UpdateAllowedAppsResult{
			Success: false,
			Message: "验证文件所有权失败",
		}, err
	}

	if !owned {
		return &UpdateAllowedAppsResult{
			Success: false,
			Message: "无权修改此文件",
		}, fmt.Errorf("无权修改此文件")
	}

	// 更新文件的允许访问应用列表
	err = s.dbService.UpdateFileAllowedApps(req.FileId, userId, req.AllowedApps)
	if err != nil {
		return &UpdateAllowedAppsResult{
			Success: false,
			Message: "更新失败",
		}, err
	}

	return &UpdateAllowedAppsResult{
		Success: true,
		Message: "更新成功",
	}, nil
}

// ParseAllowedAppsFromString 从字符串解析allowed_apps（支持JSON数组或逗号分隔）
func ParseAllowedAppsFromString(allowedAppsStr string) interface{} {
	if allowedAppsStr == "" {
		return nil
	}

	// 尝试解析JSON数组
	var apps []interface{}
	if err := json.Unmarshal([]byte(allowedAppsStr), &apps); err == nil {
		return apps
	}

	// 如果不是JSON，尝试按逗号分隔
	appsList := strings.Split(allowedAppsStr, ",")
	var appsArray []interface{}
	for _, app := range appsList {
		app = strings.TrimSpace(app)
		if app != "" {
			appsArray = append(appsArray, app)
		}
	}
	if len(appsArray) > 0 {
		return appsArray
	}

	return nil
}

// IsNFSOnline 检查是否已挂载可用的本地基础目录（用于 NFS 挂载场景）
// 返回 true 表示 [UserFiles].local_base_dir 或环境变量 USERFILES_LOCAL_BASEDIR 指向的目录可用
// NFS 可用性检查已移除

// EnsureLocalUserFilesDir 已移除：不再使用 userId 创建目录，改为使用 userId+uuid 直接创建文件

