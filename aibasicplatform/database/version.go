package database

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// VersionInfo 版本信息结构体
type VersionInfo struct {
	ID                   int       `json:"id"`
	Version              string    `json:"version"`
	Platform             string    `json:"platform"`
	IsForceUpdate        int       `json:"is_force_update"`
	DownloadURL          string    `json:"download_url"`
	FileSize             *int64    `json:"file_size,omitempty"`
	FileHash             *string   `json:"file_hash,omitempty"`
	HashType             *string   `json:"hash_type,omitempty"`
	ReleaseNotes         *string   `json:"release_notes,omitempty"`
	MinSupportedVersion  *string   `json:"min_supported_version,omitempty"`
	Status               int       `json:"status"`
	Priority             int       `json:"priority"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// CompareVersion 比较版本号
// 返回: (需要更新, 服务器版本信息, 错误)
func (s *AIBasicPlatformDataService) CompareVersion(clientVersion string, platform string) (bool, *VersionInfo, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("版本比较异常: %v", r)
		}
	}()

	// 查询指定平台的最新版本（按优先级和创建时间排序）
	query := fmt.Sprintf(`
		SELECT 
			id, version, platform, is_force_update, download_url, 
			file_size, file_hash, hash_type, release_notes, 
			min_supported_version, status, priority, created_at, updated_at
		FROM aibasicplatform.aibasicplatform_version
		WHERE platform IN ('%s', 'All') AND status = 1
		ORDER BY priority DESC, created_at DESC
		LIMIT 1
	`, platform)

	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		return false, nil, fmt.Errorf("查询版本信息失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		// 没有找到版本信息，不需要更新
		return false, nil, nil
	}

	// 解析版本信息
	versionInfo := parseVersionInfo(rows[0])
	if versionInfo == nil {
		return false, nil, fmt.Errorf("解析版本信息失败")
	}

	log.Printf("版本比较: 客户端版本=%s, 服务器版本=%s, 平台=%s", clientVersion, versionInfo.Version, platform)

	// 检查最低支持版本
	if versionInfo.MinSupportedVersion != nil && *versionInfo.MinSupportedVersion != "" {
		log.Printf("检查最低支持版本: 客户端版本=%s, 最低支持版本=%s", clientVersion, *versionInfo.MinSupportedVersion)
		if needsUpdate := compareVersionStrings(clientVersion, *versionInfo.MinSupportedVersion); needsUpdate {
			// 客户端版本低于最低支持版本，必须更新
			log.Printf("客户端版本低于最低支持版本，需要更新")
			return true, versionInfo, nil
		}
	}

	// 比较客户端版本和服务器版本
	needsUpdate := compareVersionStrings(clientVersion, versionInfo.Version)
	log.Printf("版本比较结果: 客户端版本=%s, 服务器版本=%s, 需要更新=%v", clientVersion, versionInfo.Version, needsUpdate)

	return needsUpdate, versionInfo, nil
}

// GetLatestVersion 获取指定平台的最新版本信息
func (s *AIBasicPlatformDataService) GetLatestVersion(platform string) (*VersionInfo, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取最新版本异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT 
			id, version, platform, is_force_update, download_url, 
			file_size, file_hash, hash_type, release_notes, 
			min_supported_version, status, priority, created_at, updated_at
		FROM aibasicplatform.aibasicplatform_version
		WHERE platform IN ('%s', 'All') AND status = 1
		ORDER BY priority DESC, created_at DESC
		LIMIT 1
	`, platform)

	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询版本信息失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, nil
	}

	versionInfo := parseVersionInfo(rows[0])
	return versionInfo, nil
}

// parseVersionInfo 解析版本信息
func parseVersionInfo(row map[string]interface{}) *VersionInfo {
	versionInfo := &VersionInfo{}

	if row["id"] != nil {
		versionInfo.ID = getIntValue(row["id"])
	}

	if version, ok := row["version"].(string); ok {
		versionInfo.Version = version
	}

	if platform, ok := row["platform"].(string); ok {
		versionInfo.Platform = platform
	}

	if row["is_force_update"] != nil {
		versionInfo.IsForceUpdate = getIntValue(row["is_force_update"])
	}

	if downloadURL, ok := row["download_url"].(string); ok {
		versionInfo.DownloadURL = downloadURL
	}

	if fileSize, ok := getInt64Value(row["file_size"]); ok {
		versionInfo.FileSize = &fileSize
	}

	if fileHash, ok := row["file_hash"].(string); ok && fileHash != "" {
		versionInfo.FileHash = &fileHash
	}

	if hashType, ok := row["hash_type"].(string); ok && hashType != "" {
		versionInfo.HashType = &hashType
	}

	if releaseNotes, ok := row["release_notes"].(string); ok && releaseNotes != "" {
		versionInfo.ReleaseNotes = &releaseNotes
	}

	if minSupportedVersion, ok := row["min_supported_version"].(string); ok && minSupportedVersion != "" {
		versionInfo.MinSupportedVersion = &minSupportedVersion
	}

	if row["status"] != nil {
		versionInfo.Status = getIntValue(row["status"])
	}

	if row["priority"] != nil {
		versionInfo.Priority = getIntValue(row["priority"])
	}

	if createdAt, ok := parseTime(row["created_at"]); ok {
		versionInfo.CreatedAt = createdAt
	}

	if updatedAt, ok := parseTime(row["updated_at"]); ok {
		versionInfo.UpdatedAt = updatedAt
	}

	return versionInfo
}

// compareVersionStrings 比较两个版本号字符串
// 支持格式: "1.0.5", "1.0.5+11062025", "1.0.5-beta" 等
// 返回: true 表示 clientVersion < serverVersion (需要更新)
func compareVersionStrings(clientVersion, serverVersion string) bool {
	// 提取主版本号（去掉构建号和后缀）
	clientMain := extractMainVersion(clientVersion)
	serverMain := extractMainVersion(serverVersion)

	log.Printf("版本字符串比较: 客户端完整版本=%s, 服务器完整版本=%s", clientVersion, serverVersion)
	log.Printf("版本字符串比较: 客户端主版本=%s, 服务器主版本=%s", clientMain, serverMain)

	// 比较主版本号
	clientParts := strings.Split(clientMain, ".")
	serverParts := strings.Split(serverMain, ".")

	maxLen := len(clientParts)
	if len(serverParts) > maxLen {
		maxLen = len(serverParts)
	}

	for i := 0; i < maxLen; i++ {
		var clientNum, serverNum int

		if i < len(clientParts) {
			fmt.Sscanf(clientParts[i], "%d", &clientNum)
		}

		if i < len(serverParts) {
			fmt.Sscanf(serverParts[i], "%d", &serverNum)
		}

		log.Printf("版本号部分比较 [%d]: 客户端=%d, 服务器=%d", i, clientNum, serverNum)

		if clientNum < serverNum {
			log.Printf("客户端版本号部分 [%d] 小于服务器版本，需要更新", i)
			return true // 需要更新
		} else if clientNum > serverNum {
			log.Printf("客户端版本号部分 [%d] 大于服务器版本，不需要更新", i)
			return false // 不需要更新
		}
	}

	// 主版本号相同，比较完整版本号（包含构建号）
	// 如果服务器版本包含构建号而客户端没有，或者服务器构建号更大，则需要更新
	clientBuild := extractBuildNumber(clientVersion)
	serverBuild := extractBuildNumber(serverVersion)

	log.Printf("构建号比较: 客户端构建号=%s, 服务器构建号=%s", clientBuild, serverBuild)

	if serverBuild != "" && (clientBuild == "" || clientBuild < serverBuild) {
		log.Printf("服务器构建号更大或客户端无构建号，需要更新")
		return true
	}

	log.Printf("版本相同或客户端更新，不需要更新")
	return false // 版本相同或客户端更新
}

// extractMainVersion 提取主版本号（去掉构建号和后缀）
func extractMainVersion(version string) string {
	// 去掉构建号（+后面的内容）
	if idx := strings.Index(version, "+"); idx != -1 {
		version = version[:idx]
	}
	// 去掉后缀（-后面的内容）
	if idx := strings.Index(version, "-"); idx != -1 {
		version = version[:idx]
	}
	return strings.TrimSpace(version)
}

// extractBuildNumber 提取构建号
func extractBuildNumber(version string) string {
	if idx := strings.Index(version, "+"); idx != -1 {
		return strings.TrimSpace(version[idx+1:])
	}
	return ""
}

// integer helpers have been moved to helpers.go

// parseTime 解析时间
func parseTime(val interface{}) (time.Time, bool) {
	switch v := val.(type) {
	case time.Time:
		return v, true
	case string:
		if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
			return t, true
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t, true
		}
		return time.Time{}, false
	default:
		return time.Time{}, false
	}
}

