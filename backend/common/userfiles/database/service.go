package database

import (
	"encoding/json"
	"fmt"
	"log"

	"digitalsingularity/backend/common/utils/datahandle"
)

// UserFileService 用户文件服务
type UserFileService struct {
	readWrite *datahandle.CommonReadWriteService
}

// NewUserFileService 创建一个新的用户文件服务实例
func NewUserFileService() *UserFileService {
	rwService, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		log.Printf("创建用户文件服务实例错误: %v", err)
		return &UserFileService{}
	}

	return &UserFileService{
		readWrite: rwService,
	}
}

// CreateFileRecord 创建文件记录
func (s *UserFileService) CreateFileRecord(data map[string]interface{}) (map[string]interface{}, error) {
	query := `
		INSERT INTO common.user_files 
		(user_id, file_id, original_name, file_type, mime_type, file_size, file_path, file_hash, upload_status, chunk_total, chunk_uploaded, allowed_apps)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	userId := data["user_id"].(string)
	fileId := data["file_id"].(string)
	originalName := data["original_name"].(string)
	fileType := data["file_type"].(string)
	mimeType := data["mime_type"]
	fileSize := data["file_size"]
	filePath := data["file_path"].(string)
	var fileHash interface{} = nil
	if hash, ok := data["file_hash"]; ok && hash != nil {
		if hashStr, ok := hash.(string); ok && hashStr != "" {
			fileHash = hashStr
		}
	}
	uploadStatus := "uploading"
	if status, ok := data["upload_status"].(string); ok {
		uploadStatus = status
	}
	chunkTotal := 1
	if ct, ok := data["chunk_total"].(int); ok {
		chunkTotal = ct
	}
	chunkUploaded := 0
	if cu, ok := data["chunk_uploaded"].(int); ok {
		chunkUploaded = cu
	}
	
	// 处理allowed_apps字段（JSON数组）
	var allowedAppsJSON interface{} = nil
	if allowedApps, ok := data["allowed_apps"]; ok && allowedApps != nil {
		// 如果已经是字符串（JSON格式），直接使用
		if str, ok := allowedApps.(string); ok {
			allowedAppsJSON = str
		} else if arr, ok := allowedApps.([]interface{}); ok {
			// 如果是数组，转换为JSON字符串
			jsonBytes, err := json.Marshal(arr)
			if err == nil {
				allowedAppsJSON = string(jsonBytes)
			}
		} else if arr, ok := allowedApps.([]string); ok {
			// 如果是字符串数组，转换为JSON字符串
			jsonBytes, err := json.Marshal(arr)
			if err == nil {
				allowedAppsJSON = string(jsonBytes)
			}
		}
	}

	opResult := s.readWrite.ExecuteDb(query, userId, fileId, originalName, fileType, mimeType, fileSize, filePath, fileHash, uploadStatus, chunkTotal, chunkUploaded, allowedAppsJSON)
	if !opResult.IsSuccess() {
		log.Printf("创建文件记录错误: %v", opResult.Error)
		return nil, opResult.Error
	}

	// 查询创建的文件记录（不指定应用ID，因为刚创建的文件所有者可以访问）
	fileRecord, err := s.GetFileByFileId(fileId, "")
	if err != nil {
		return nil, err
	}

	// 同步到Redis
	s.syncFileToRedis(userId, fileId, fileRecord)

	return fileRecord, nil
}

// GetFileByFileId 根据文件ID获取文件信息
// appId: 应用ID，用于权限验证。如果为空字符串，则不进行应用权限验证（仅用于文件所有者）
func (s *UserFileService) GetFileByFileId(fileId string, appId string) (map[string]interface{}, error) {
	query := `
		SELECT id, user_id, file_id, original_name, file_type, mime_type, file_size, 
			file_path, file_hash, upload_status, chunk_total, chunk_uploaded, allowed_apps, created_at, updated_at
		FROM common.user_files
		WHERE file_id = ? AND deleted_at IS NULL
	`

	opResult := s.readWrite.QueryDb(query, fileId)
	if !opResult.IsSuccess() {
		log.Printf("获取文件信息错误: %v", opResult.Error)
		return nil, opResult.Error
	}

	results, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(results) == 0 {
		return nil, fmt.Errorf("文件不存在")
	}

	fileRecord := results[0]
	
	// 如果指定了appId，进行应用权限验证
	if appId != "" {
		hasAccess, err := s.checkAppAccess(fileRecord, appId)
		if err != nil {
			return nil, err
		}
		if !hasAccess {
			return nil, fmt.Errorf("当前应用无权访问此文件")
		}
	}

	return fileRecord, nil
}

// GetFileByHash 根据文件MD5哈希值获取文件信息（用于去重）
// 只查询同一用户的文件，且文件状态为completed
func (s *UserFileService) GetFileByHash(userId string, fileHash string) (map[string]interface{}, error) {
	query := `
		SELECT id, user_id, file_id, original_name, file_type, mime_type, file_size, 
			file_path, file_hash, upload_status, chunk_total, chunk_uploaded, allowed_apps, created_at, updated_at
		FROM common.user_files
		WHERE user_id = ? AND file_hash = ? AND upload_status = 'completed' AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	opResult := s.readWrite.QueryDb(query, userId, fileHash)
	if !opResult.IsSuccess() {
		log.Printf("根据MD5获取文件信息错误: %v", opResult.Error)
		return nil, opResult.Error
	}

	results, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(results) == 0 {
		return nil, fmt.Errorf("文件不存在")
	}

	return results[0], nil
}

// checkAppAccess 检查应用是否有权限访问文件
// 如果allowed_apps为NULL，表示所有应用都可以访问
// 如果allowed_apps不为NULL，则检查appId是否在列表中
func (s *UserFileService) checkAppAccess(fileRecord map[string]interface{}, appId string) (bool, error) {
	allowedApps := fileRecord["allowed_apps"]
	
	// 如果allowed_apps为NULL，所有应用都可以访问
	if allowedApps == nil {
		return true, nil
	}
	
	// 解析JSON数组
	var appIds []string
	switch v := allowedApps.(type) {
	case string:
		// 如果是字符串，尝试解析JSON
		if v == "" || v == "null" {
			return true, nil
		}
		if err := json.Unmarshal([]byte(v), &appIds); err != nil {
			log.Printf("解析allowed_apps JSON错误: %v", err)
			return false, fmt.Errorf("文件权限配置错误")
		}
	case []interface{}:
		// 如果已经是数组，直接转换
		for _, item := range v {
			if str, ok := item.(string); ok {
				appIds = append(appIds, str)
			}
		}
	case []string:
		appIds = v
	default:
		log.Printf("allowed_apps类型不支持: %T", v)
		return false, fmt.Errorf("文件权限配置错误")
	}
	
	// 检查appId是否在列表中
	for _, id := range appIds {
		if id == appId {
			return true, nil
		}
	}
	
	return false, nil
}

// GetUserFiles 获取用户的所有文件列表
// appId: 应用ID，用于过滤文件。如果为空字符串，则返回所有文件（仅用于文件所有者）
func (s *UserFileService) GetUserFiles(userId string, appId string) ([]map[string]interface{}, error) {
	query := `
		SELECT id, user_id, file_id, original_name, file_type, mime_type, file_size, 
			file_path, file_hash, upload_status, chunk_total, chunk_uploaded, allowed_apps, created_at, updated_at
		FROM common.user_files
		WHERE user_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	opResult := s.readWrite.QueryDb(query, userId)
	if !opResult.IsSuccess() {
		log.Printf("获取用户文件列表错误: %v", opResult.Error)
		return nil, opResult.Error
	}

	results, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	// 如果指定了appId，过滤文件
	if appId != "" {
		var filteredResults []map[string]interface{}
		for _, fileRecord := range results {
			hasAccess, err := s.checkAppAccess(fileRecord, appId)
			if err != nil {
				log.Printf("检查文件访问权限错误: %v", err)
				continue
			}
			if hasAccess {
				filteredResults = append(filteredResults, fileRecord)
			}
		}
		return filteredResults, nil
	}

	return results, nil
}

// UpdateFileStatus 更新文件上传状态
func (s *UserFileService) UpdateFileStatus(fileId string, status string, chunkUploaded int) error {
	query := `
		UPDATE common.user_files
		SET upload_status = ?, chunk_uploaded = ?, updated_at = NOW()
		WHERE file_id = ? AND deleted_at IS NULL
	`

	opResult := s.readWrite.ExecuteDb(query, status, chunkUploaded, fileId)
	if !opResult.IsSuccess() {
		log.Printf("更新文件状态错误: %v", opResult.Error)
		return opResult.Error
	}

	return nil
}

// UpdateFileStatusWithHash 更新文件上传状态和MD5哈希值
func (s *UserFileService) UpdateFileStatusWithHash(fileId string, status string, chunkUploaded int, fileHash string) error {
	query := `
		UPDATE common.user_files
		SET upload_status = ?, chunk_uploaded = ?, file_hash = ?, updated_at = NOW()
		WHERE file_id = ? AND deleted_at IS NULL
	`

	opResult := s.readWrite.ExecuteDb(query, status, chunkUploaded, fileHash, fileId)
	if !opResult.IsSuccess() {
		log.Printf("更新文件状态和MD5错误: %v", opResult.Error)
		return opResult.Error
	}

	// 如果状态为completed，同步到Redis
	if status == "completed" {
		// 获取完整的文件信息
		fileRecord, err := s.GetFileByFileId(fileId, "")
		if err == nil && fileRecord != nil {
			if userId, ok := fileRecord["user_id"].(string); ok {
				s.syncFileToRedis(userId, fileId, fileRecord)
			}
		}
	}

	return nil
}

// DeleteFile 软删除文件（设置deleted_at）
func (s *UserFileService) DeleteFile(fileId string, userId string) error {
	query := `
		UPDATE common.user_files
		SET deleted_at = NOW()
		WHERE file_id = ? AND user_id = ? AND deleted_at IS NULL
	`

	opResult := s.readWrite.ExecuteDb(query, fileId, userId)
	if !opResult.IsSuccess() {
		log.Printf("删除文件错误: %v", opResult.Error)
		return opResult.Error
	}

	// 从Redis中删除
	s.deleteFileFromRedis(userId, fileId)

	return nil
}

// VerifyFileOwnership 验证文件所有权
func (s *UserFileService) VerifyFileOwnership(fileId string, userId string) (bool, error) {
	query := `
		SELECT COUNT(*) as count
		FROM common.user_files
		WHERE file_id = ? AND user_id = ? AND deleted_at IS NULL
	`

	opResult := s.readWrite.QueryDb(query, fileId, userId)
	if !opResult.IsSuccess() {
		log.Printf("验证文件所有权错误: %v", opResult.Error)
		return false, opResult.Error
	}

	results, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(results) == 0 {
		return false, nil
	}

	count, ok := results[0]["count"]
	if !ok {
		return false, nil
	}

	// 处理不同的数字类型
	var countInt int64
	switch v := count.(type) {
	case int64:
		countInt = v
	case int:
		countInt = int64(v)
	case float64:
		countInt = int64(v)
	default:
		return false, nil
	}

	return countInt > 0, nil
}

// UpdateFileAllowedApps 更新文件的允许访问应用列表
func (s *UserFileService) UpdateFileAllowedApps(fileId string, userId string, allowedApps interface{}) error {
	var allowedAppsJSON interface{} = nil
	if allowedApps != nil {
		// 如果已经是字符串（JSON格式），直接使用
		if str, ok := allowedApps.(string); ok {
			allowedAppsJSON = str
		} else if arr, ok := allowedApps.([]interface{}); ok {
			// 如果是数组，转换为JSON字符串
			jsonBytes, err := json.Marshal(arr)
			if err != nil {
				return fmt.Errorf("转换allowed_apps为JSON失败: %v", err)
			}
			allowedAppsJSON = string(jsonBytes)
		} else if arr, ok := allowedApps.([]string); ok {
			// 如果是字符串数组，转换为JSON字符串
			jsonBytes, err := json.Marshal(arr)
			if err != nil {
				return fmt.Errorf("转换allowed_apps为JSON失败: %v", err)
			}
			allowedAppsJSON = string(jsonBytes)
		}
	}

	query := `
		UPDATE common.user_files
		SET allowed_apps = ?, updated_at = NOW()
		WHERE file_id = ? AND user_id = ? AND deleted_at IS NULL
	`

	opResult := s.readWrite.ExecuteDb(query, allowedAppsJSON, fileId, userId)
	if !opResult.IsSuccess() {
		log.Printf("更新文件允许访问应用列表错误: %v", opResult.Error)
		return opResult.Error
	}

	// 同步到Redis
	fileRecord, err := s.GetFileByFileId(fileId, "")
	if err == nil && fileRecord != nil {
		s.syncFileToRedis(userId, fileId, fileRecord)
	}

	return nil
}

// syncFileToRedis 将文件信息同步到Redis（永久有效）
func (s *UserFileService) syncFileToRedis(userId string, fileId string, fileData map[string]interface{}) {
	if s.readWrite == nil {
		return // Redis未初始化，静默跳过
	}

	// 构建Redis key: user_file:{userId}:{fileId}
	redisKey := fmt.Sprintf("user_file:%s:%s", userId, fileId)

	// 将文件信息转换为JSON
	fileJson, err := json.Marshal(fileData)
	if err != nil {
		log.Printf("序列化文件信息到JSON失败: %v", err)
		return
	}

	// 写入Redis，expire为0表示永不过期
	result := s.readWrite.SetRedis(redisKey, string(fileJson), 0)
	if !result.IsSuccess() {
		log.Printf("同步文件信息到Redis失败 (key: %s): %v", redisKey, result.Error)
		return
	}

	log.Printf("文件信息已同步到Redis: %s", redisKey)
}

// deleteFileFromRedis 从Redis中删除文件信息
func (s *UserFileService) deleteFileFromRedis(userId string, fileId string) {
	if s.readWrite == nil {
		return // Redis未初始化，静默跳过
	}

	// 构建Redis key: user_file:{userId}:{fileId}
	redisKey := fmt.Sprintf("user_file:%s:%s", userId, fileId)

	// 从Redis中删除
	result := s.readWrite.DeleteRedis(redisKey)
	if !result.IsSuccess() {
		log.Printf("从Redis删除文件信息失败 (key: %s): %v", redisKey, result.Error)
		return
	}

	log.Printf("文件信息已从Redis删除: %s", redisKey)
}
