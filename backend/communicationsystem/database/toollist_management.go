package database

import (
	"digitalsingularity/backend/common/utils/datahandle"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	commonService *datahandle.CommonReadWriteService
	commonOnce    sync.Once
)

// getCommonService 获取通用数据库服务实例（使用单例模式）
func getCommonService() (*datahandle.CommonReadWriteService, error) {
	var err error
	commonOnce.Do(func() {
		commonService, err = datahandle.NewCommonReadWriteService("communication_system")
	})
	return commonService, err
}

// CloseCommonService 关闭通用数据库连接
func CloseCommonService() {
	if commonService != nil {
		commonService.Close()
		commonService = nil
	}
}

// Tool 工具数据结构
type Tool struct {
	ID          string `json:"id"`
	NameZh      string `json:"name_zh"`
	NameEn      string `json:"name_en"`
	DescriptionZh string `json:"description_zh"`
	DescriptionEn string `json:"description_en"`
	DetailsZh   string `json:"details_zh"`
	DetailsEn   string `json:"details_en"`
	Icon        string `json:"icon"`
	IconPathLight string `json:"icon_path_light,omitempty"`
	IconPathDark string `json:"icon_path_dark,omitempty"`
	CategoryZh  string `json:"category_zh"`
	CategoryEn  string `json:"category_en"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	FeaturesZh  []string `json:"features_zh"`
	FeaturesEn  []string `json:"features_en"`
	UsageZh     string `json:"usage_zh"`
	UsageEn     string `json:"usage_en"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// GetToolList 从Redis获取所有启用的工具列表，如果Redis没有数据则从数据库读取
func GetToolList() ([]Tool, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据服务失败: %v", err)
	}

	var tools []Tool

	// 先尝试获取已知的工具ID
	knownToolIDs := []string{"potagi_assistant", "semantic_search_system"}

	// 尝试从Redis读取所有工具
	redisTools := 0
	for _, toolID := range knownToolIDs {
		key := fmt.Sprintf("tool:%s", toolID)

		// 从Redis读取工具信息
		result := service.RedisRead(key)
		if !result.IsSuccess() {
			log.Printf("从Redis读取工具 %s 失败: %v", toolID, result.Error)
			continue
		}

		var toolInfo map[string]interface{}

		// 处理数据格式
		switch data := result.Data.(type) {
		case string:
			// 如果是字符串，需要解析JSON
			if data == "" {
				continue
			}
			if err := json.Unmarshal([]byte(data), &toolInfo); err != nil {
				log.Printf("工具 %s 的数据不是有效的JSON格式: %v", toolID, err)
				continue
			}
		case map[string]interface{}:
			// 如果已经是map类型，直接使用
			toolInfo = data
		default:
			log.Printf("工具 %s 的数据类型不支持: %T", toolID, result.Data)
			continue
		}

		// 转换为Tool结构体
		tool := Tool{
			ID: toolID,
		}

		// 安全地提取各个字段
		if val, ok := toolInfo["name_zh"].(string); ok {
			tool.NameZh = val
		}
		if val, ok := toolInfo["name_en"].(string); ok {
			tool.NameEn = val
		}
		if val, ok := toolInfo["description_zh"].(string); ok {
			tool.DescriptionZh = val
		}
		if val, ok := toolInfo["description_en"].(string); ok {
			tool.DescriptionEn = val
		}
		if val, ok := toolInfo["details_zh"].(string); ok {
			tool.DetailsZh = val
		}
		if val, ok := toolInfo["details_en"].(string); ok {
			tool.DetailsEn = val
		}
		if val, ok := toolInfo["icon"].(string); ok {
			tool.Icon = val
		}
		if val, ok := toolInfo["icon_path_light"].(string); ok {
			tool.IconPathLight = val
		}
		if val, ok := toolInfo["icon_path_dark"].(string); ok {
			tool.IconPathDark = val
		}
		if val, ok := toolInfo["category_zh"].(string); ok {
			tool.CategoryZh = val
		}
		if val, ok := toolInfo["category_en"].(string); ok {
			tool.CategoryEn = val
		}
		if val, ok := toolInfo["version"].(string); ok {
			tool.Version = val
		}
		if val, ok := toolInfo["author"].(string); ok {
			tool.Author = val
		}

		// 处理features数组
		if featuresZh, ok := toolInfo["features_zh"].([]interface{}); ok {
			tool.FeaturesZh = make([]string, len(featuresZh))
			for i, f := range featuresZh {
				if str, ok := f.(string); ok {
					tool.FeaturesZh[i] = str
				}
			}
		}
		if featuresEn, ok := toolInfo["features_en"].([]interface{}); ok {
			tool.FeaturesEn = make([]string, len(featuresEn))
			for i, f := range featuresEn {
				if str, ok := f.(string); ok {
					tool.FeaturesEn[i] = str
				}
			}
		}

		if val, ok := toolInfo["usage_zh"].(string); ok {
			tool.UsageZh = val
		}
		if val, ok := toolInfo["usage_en"].(string); ok {
			tool.UsageEn = val
		}
		if val, ok := toolInfo["enabled"].(bool); ok {
			tool.Enabled = val
		}
		if val, ok := toolInfo["sort_order"].(float64); ok {
			tool.SortOrder = int(val)
		}
		if val, ok := toolInfo["created_at"].(string); ok {
			tool.CreatedAt = val
		}
		if val, ok := toolInfo["updated_at"].(string); ok {
			tool.UpdatedAt = val
		}

		// 只添加启用的工具
		if tool.Enabled {
			tools = append(tools, tool)
			redisTools++
		}
	}

	// 如果Redis中没有找到所有工具，从数据库查询
	if redisTools == 0 {
		log.Println("Redis中没有找到工具数据，从数据库查询")
		return getToolsFromDatabase(service)
	}

	// 按sort_order排序
	for i := 0; i < len(tools)-1; i++ {
		for j := i + 1; j < len(tools); j++ {
			if tools[i].SortOrder > tools[j].SortOrder {
				tools[i], tools[j] = tools[j], tools[i]
			}
		}
	}

	return tools, nil
}

// GetToolByID 根据ID获取单个工具
func GetToolByID(toolID string) (*Tool, error) {
	service, err := getService()
	if err != nil {
		return nil, fmt.Errorf("获取数据服务失败: %v", err)
	}

	key := fmt.Sprintf("tool:%s", toolID)

	// 从Redis读取工具信息
	result := service.RedisRead(key)
	if !result.IsSuccess() {
		if result.Status == datahandle.StatusNotFound {
			return nil, fmt.Errorf("工具不存在")
		}
		return nil, fmt.Errorf("从Redis读取工具失败: %v", result.Error)
	}

	var toolInfo map[string]interface{}

	// 处理数据格式
	switch data := result.Data.(type) {
	case string:
		// 如果是字符串，需要解析JSON
		if data == "" {
			return nil, fmt.Errorf("工具数据为空")
		}
		if err := json.Unmarshal([]byte(data), &toolInfo); err != nil {
			return nil, fmt.Errorf("工具数据JSON格式错误: %v", err)
		}
	case map[string]interface{}:
		// 如果已经是map类型，直接使用
		toolInfo = data
	default:
		return nil, fmt.Errorf("工具数据类型不支持: %T", result.Data)
	}

	// 转换为Tool结构体
	tool := &Tool{
		ID: toolID,
	}

	// 安全地提取各个字段
	if val, ok := toolInfo["name_zh"].(string); ok {
		tool.NameZh = val
	}
	if val, ok := toolInfo["name_en"].(string); ok {
		tool.NameEn = val
	}
	if val, ok := toolInfo["description_zh"].(string); ok {
		tool.DescriptionZh = val
	}
	if val, ok := toolInfo["description_en"].(string); ok {
		tool.DescriptionEn = val
	}
	if val, ok := toolInfo["details_zh"].(string); ok {
		tool.DetailsZh = val
	}
	if val, ok := toolInfo["details_en"].(string); ok {
		tool.DetailsEn = val
	}
	if val, ok := toolInfo["icon"].(string); ok {
		tool.Icon = val
	}
	if val, ok := toolInfo["icon_path_light"].(string); ok {
		tool.IconPathLight = val
	}
	if val, ok := toolInfo["icon_path_dark"].(string); ok {
		tool.IconPathDark = val
	}
	if val, ok := toolInfo["category_zh"].(string); ok {
		tool.CategoryZh = val
	}
	if val, ok := toolInfo["category_en"].(string); ok {
		tool.CategoryEn = val
	}
	if val, ok := toolInfo["version"].(string); ok {
		tool.Version = val
	}
	if val, ok := toolInfo["author"].(string); ok {
		tool.Author = val
	}

	// 处理features数组
	if featuresZh, ok := toolInfo["features_zh"].([]interface{}); ok {
		tool.FeaturesZh = make([]string, len(featuresZh))
		for i, f := range featuresZh {
			if str, ok := f.(string); ok {
				tool.FeaturesZh[i] = str
			}
		}
	}
	if featuresEn, ok := toolInfo["features_en"].([]interface{}); ok {
		tool.FeaturesEn = make([]string, len(featuresEn))
		for i, f := range featuresEn {
			if str, ok := f.(string); ok {
				tool.FeaturesEn[i] = str
			}
		}
	}

	if val, ok := toolInfo["usage_zh"].(string); ok {
		tool.UsageZh = val
	}
	if val, ok := toolInfo["usage_en"].(string); ok {
		tool.UsageEn = val
	}
	if val, ok := toolInfo["enabled"].(bool); ok {
		tool.Enabled = val
	}
	if val, ok := toolInfo["sort_order"].(float64); ok {
		tool.SortOrder = int(val)
	}
	if val, ok := toolInfo["created_at"].(string); ok {
		tool.CreatedAt = val
	}
	if val, ok := toolInfo["updated_at"].(string); ok {
		tool.UpdatedAt = val
	}

	return tool, nil
}

// CreateTool 创建新工具（支持单个）
func CreateTool(tool Tool) error {
	return CreateTools([]Tool{tool})
}

// CreateTools 批量创建工具
func CreateTools(tools []Tool) error {
	service, err := getCommonService()
	if err != nil {
		return fmt.Errorf("获取数据服务失败: %v", err)
	}

	currentTime := time.Now().Format("2006-01-02 15:04:05")

	for _, tool := range tools {
		// 检查工具是否已存在
		key := fmt.Sprintf("tool:%s", tool.ID)
		existing := service.RedisRead(key)
		if existing.IsSuccess() {
			return fmt.Errorf("工具已存在: %s", tool.ID)
		}

		// 设置创建和更新时间
		tool.CreatedAt = currentTime
		tool.UpdatedAt = currentTime

		// 存储到Redis
		result := service.RedisWrite(key, tool, 0) // 0表示不过期
		if !result.IsSuccess() {
			return fmt.Errorf("保存工具到Redis失败: %v", result.Error)
		}
	}

	return nil
}

// UpdateTool 更新工具信息（支持单个）
func UpdateTool(tool Tool) error {
	return UpdateTools([]Tool{tool})
}

// UpdateTools 批量更新工具
func UpdateTools(tools []Tool) error {
	service, err := getCommonService()
	if err != nil {
		return fmt.Errorf("获取数据服务失败: %v", err)
	}

	currentTime := time.Now().Format("2006-01-02 15:04:05")

	for _, tool := range tools {
		// 检查工具是否存在
		key := fmt.Sprintf("tool:%s", tool.ID)
		existing := service.RedisRead(key)
		if !existing.IsSuccess() {
			return fmt.Errorf("工具不存在: %s", tool.ID)
		}

		// 更新时间戳
		tool.UpdatedAt = currentTime

		// 保留创建时间
		if existingTool, err := GetToolByID(tool.ID); err == nil && existingTool.CreatedAt != "" {
			tool.CreatedAt = existingTool.CreatedAt
		}

		// 更新Redis中的数据
		result := service.RedisWrite(key, tool, 0)
		if !result.IsSuccess() {
			return fmt.Errorf("更新工具到Redis失败: %v", result.Error)
		}
	}

	return nil
}

// DeleteTool 删除工具（支持单个）
func DeleteTool(toolID string) error {
	return DeleteTools([]string{toolID})
}

// DeleteTools 批量删除工具
func DeleteTools(toolIDs []string) error {
	service, err := getCommonService()
	if err != nil {
		return fmt.Errorf("获取数据服务失败: %v", err)
	}

	for _, toolID := range toolIDs {
		// 检查工具是否存在
		key := fmt.Sprintf("tool:%s", toolID)
		existing := service.RedisRead(key)
		if !existing.IsSuccess() {
			return fmt.Errorf("工具不存在: %s", toolID)
		}

		// 从Redis删除
		result := service.DeleteRedis(key)
		if !result.IsSuccess() {
			return fmt.Errorf("从Redis删除工具失败: %v", result.Error)
		}
	}

	return nil
}

// GetToolsByCategory 根据分类获取工具
func GetToolsByCategory(categoryZh string) ([]Tool, error) {
	allTools, err := GetToolList()
	if err != nil {
		return nil, err
	}

	var filteredTools []Tool
	for _, tool := range allTools {
		if tool.CategoryZh == categoryZh && tool.Enabled {
			filteredTools = append(filteredTools, tool)
		}
	}

	return filteredTools, nil
}

// getToolsFromDatabase 从数据库查询所有启用的工具，并同步到Redis
func getToolsFromDatabase(service *datahandle.CommonReadWriteService) ([]Tool, error) {
	// 查询数据库中的所有启用工具
	query := `
		SELECT id, name_zh, name_en, description_zh, description_en, details_zh, details_en,
			   icon, icon_path_light, icon_path_dark, category_zh, category_en, version, author,
			   features_zh, features_en, usage_zh, usage_en, enabled, sort_order, created_at, updated_at
		FROM tools
		WHERE enabled = 1
		ORDER BY sort_order ASC
	`

	result := service.QueryDb(query)
	if !result.IsSuccess() {
		return nil, fmt.Errorf("从数据库查询工具失败: %v", result.Error)
	}

	var tools []Tool

	// 处理查询结果
	switch rows := result.Data.(type) {
	case []map[string]interface{}:
		for _, row := range rows {
			tool := Tool{}

			// 基本字段
			if val, ok := row["id"].(string); ok {
				tool.ID = val
			}
			if val, ok := row["name_zh"].(string); ok {
				tool.NameZh = val
			}
			if val, ok := row["name_en"].(string); ok {
				tool.NameEn = val
			}
			if val, ok := row["description_zh"].(string); ok {
				tool.DescriptionZh = val
			}
			if val, ok := row["description_en"].(string); ok {
				tool.DescriptionEn = val
			}
			if val, ok := row["details_zh"].(string); ok {
				tool.DetailsZh = val
			}
			if val, ok := row["details_en"].(string); ok {
				tool.DetailsEn = val
			}
			if val, ok := row["icon"].(string); ok {
				tool.Icon = val
			}
			if val, ok := row["icon_path_light"].(string); ok {
				tool.IconPathLight = val
			}
			if val, ok := row["icon_path_dark"].(string); ok {
				tool.IconPathDark = val
			}
			if val, ok := row["category_zh"].(string); ok {
				tool.CategoryZh = val
			}
			if val, ok := row["category_en"].(string); ok {
				tool.CategoryEn = val
			}
			if val, ok := row["version"].(string); ok {
				tool.Version = val
			}
			if val, ok := row["author"].(string); ok {
				tool.Author = val
			}
			if val, ok := row["usage_zh"].(string); ok {
				tool.UsageZh = val
			}
			if val, ok := row["usage_en"].(string); ok {
				tool.UsageEn = val
			}
			if val, ok := row["created_at"].(string); ok {
				tool.CreatedAt = val
			}
			if val, ok := row["updated_at"].(string); ok {
				tool.UpdatedAt = val
			}

			// 布尔字段 - 使用辅助函数更安全地处理
			if enabledVal, ok := getInt64Value(row["enabled"]); ok {
				tool.Enabled = enabledVal == 1
				log.Printf("数据库读取工具 %s 的enabled字段: %d -> %v", tool.ID, enabledVal, tool.Enabled)
			} else {
				// 如果无法解析，默认启用
				tool.Enabled = true
				log.Printf("数据库读取工具 %s 的enabled字段解析失败，使用默认值: true", tool.ID)
			}

			// 排序字段
			if val, ok := row["sort_order"].(int64); ok {
				tool.SortOrder = int(val)
			}

			// JSON字段 - features
			if val, ok := row["features_zh"].(string); ok {
				var features []string
				if err := json.Unmarshal([]byte(val), &features); err == nil {
					tool.FeaturesZh = features
				}
			}
			if val, ok := row["features_en"].(string); ok {
				var features []string
				if err := json.Unmarshal([]byte(val), &features); err == nil {
					tool.FeaturesEn = features
				}
			}

			tools = append(tools, tool)

			// 同步到Redis（使用原始的service实例）
			toolData := map[string]interface{}{
				"id":             tool.ID,
				"name_zh":        tool.NameZh,
				"name_en":        tool.NameEn,
				"description_zh": tool.DescriptionZh,
				"description_en": tool.DescriptionEn,
				"details_zh":     tool.DetailsZh,
				"details_en":     tool.DetailsEn,
				"icon":           tool.Icon,
				"icon_path_light": tool.IconPathLight,
				"icon_path_dark": tool.IconPathDark,
				"category_zh":    tool.CategoryZh,
				"category_en":    tool.CategoryEn,
				"version":        tool.Version,
				"author":         tool.Author,
				"features_zh":    tool.FeaturesZh,
				"features_en":    tool.FeaturesEn,
				"usage_zh":       tool.UsageZh,
				"usage_en":       tool.UsageEn,
				"enabled":        tool.Enabled,
				"sort_order":     tool.SortOrder,
				"created_at":     tool.CreatedAt,
				"updated_at":     tool.UpdatedAt,
			}

			toolJSON, _ := json.Marshal(toolData)
			key := fmt.Sprintf("tool:%s", tool.ID)
			service.RedisWrite(key, string(toolJSON), 24*time.Hour) // 缓存24小时
		}
	default:
		return nil, fmt.Errorf("数据库查询结果格式错误")
	}

	log.Printf("从数据库成功加载了 %d 个工具到Redis", len(tools))
	return tools, nil
}

