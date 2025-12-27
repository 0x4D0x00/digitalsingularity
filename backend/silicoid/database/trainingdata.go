package database

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// TrainingData 训练数据结构体
type TrainingData struct {
	ID            *int     `json:"id,omitempty"`
	SessionID     string   `json:"session_id"`              // 会话ID，必填
	Conversations string   `json:"conversations"`           // LLaMA Factory格式的对话数据JSON字符串，必填
	SystemPrompt  *string  `json:"system_prompt,omitempty"` // 系统提示词，可选
	IsPositive    int      `json:"is_positive"`             // 用户反馈类型（1=点赞, 0=点踩），必填
	UserID        *string  `json:"user_id,omitempty"`       // 用户ID，可选
	FeedbackReason *string `json:"feedback_reason,omitempty"` // 反馈原因，可选
	QualityScore  *int     `json:"quality_score,omitempty"`  // 数据质量评分（0-100），可选
	Tags          *string  `json:"tags,omitempty"`          // 标签列表JSON字符串，可选
	Metadata      *string  `json:"metadata,omitempty"`      // 额外元数据JSON字符串，可选
	IsVerified    *int     `json:"is_verified,omitempty"`   // 是否已人工验证（1=是, 0=否），可选
	IsEnabled     *int     `json:"is_enabled,omitempty"`    // 是否启用（1=启用, 0=禁用），可选
}

// ==================== 创建操作 ====================

// CreateTrainingData 创建单个训练数据
// 参数:
//   data: 训练数据
// 返回: 新创建的数据ID和错误信息
func (s *SilicoidDataService) CreateTrainingData(data TrainingData) (int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("创建训练数据异常: %v", r)
		}
	}()

	// 验证必填字段
	if data.SessionID == "" {
		return 0, fmt.Errorf("session_id 不能为空")
	}
	if data.Conversations == "" {
		return 0, fmt.Errorf("conversations 不能为空")
	}

	// 验证 conversations 是否为有效的JSON
	var conversationsJSON interface{}
	if err := json.Unmarshal([]byte(data.Conversations), &conversationsJSON); err != nil {
		return 0, fmt.Errorf("conversations 必须是有效的JSON格式: %v", err)
	}

	// 设置默认值
	isEnabled := 1
	if data.IsEnabled != nil {
		isEnabled = *data.IsEnabled
	}

	isVerified := 0
	if data.IsVerified != nil {
		isVerified = *data.IsVerified
	}

	qualityScore := 0
	if data.QualityScore != nil {
		qualityScore = *data.QualityScore
	}

	// 处理可选字段
	systemPrompt := interface{}(nil)
	if data.SystemPrompt != nil {
		systemPrompt = *data.SystemPrompt
	}

	userID := interface{}(nil)
	if data.UserID != nil {
		userID = *data.UserID
	}

	feedbackReason := interface{}(nil)
	if data.FeedbackReason != nil {
		feedbackReason = *data.FeedbackReason
	}

	tags := interface{}(nil)
	if data.Tags != nil {
		// 验证 tags 是否为有效的JSON
		var tagsJSON interface{}
		if err := json.Unmarshal([]byte(*data.Tags), &tagsJSON); err != nil {
			return 0, fmt.Errorf("tags 必须是有效的JSON格式: %v", err)
		}
		tags = *data.Tags
	} else {
		tags = "[]"
	}

	metadata := interface{}(nil)
	if data.Metadata != nil {
		// 验证 metadata 是否为有效的JSON
		var metadataJSON interface{}
		if err := json.Unmarshal([]byte(*data.Metadata), &metadataJSON); err != nil {
			return 0, fmt.Errorf("metadata 必须是有效的JSON格式: %v", err)
		}
		metadata = *data.Metadata
	} else {
		metadata = "{}"
	}

	// 构建插入SQL
	query := fmt.Sprintf(`
		INSERT INTO %s.silicoid_training_dataset
		(session_id, conversations, system_prompt, is_positive, user_id,
		 feedback_reason, quality_score, tags, metadata, is_verified, is_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.dbName)

	opResult := s.readWrite.ExecuteDb(query,
		data.SessionID, data.Conversations, systemPrompt, data.IsPositive, userID,
		feedbackReason, qualityScore, tags, metadata, isVerified, isEnabled,
	)

	if !opResult.IsSuccess() {
		return 0, fmt.Errorf("创建训练数据失败: %v", opResult.Error)
	}

	// 获取插入的ID（使用LAST_INSERT_ID()更可靠，避免JSON字段比较问题）
	idQuery := fmt.Sprintf(`SELECT LAST_INSERT_ID() as id`)
	idResult := s.readWrite.QueryDb(idQuery)
	if idResult.IsSuccess() {
		if rows, ok := idResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			newID := getIntValue(rows[0]["id"])
			if newID > 0 {
				log.Printf("✅ 成功创建训练数据: id=%d, session_id=%s", newID, data.SessionID)
				return newID, nil
			}
		}
	}

	// 如果LAST_INSERT_ID()失败，尝试通过session_id和created_at查询（作为备用方案）
	// 如果提供了user_id，也在查询条件中包含它以提高准确性
	var fallbackQuery string
	var fallbackArgs []interface{}
	if data.UserID != nil && *data.UserID != "" {
		fallbackQuery = fmt.Sprintf(`
			SELECT id FROM %s.silicoid_training_dataset
			WHERE session_id = ? AND user_id = ?
			ORDER BY created_at DESC, id DESC LIMIT 1
		`, s.dbName)
		fallbackArgs = []interface{}{data.SessionID, *data.UserID}
	} else {
		fallbackQuery = fmt.Sprintf(`
			SELECT id FROM %s.silicoid_training_dataset
			WHERE session_id = ?
			ORDER BY created_at DESC, id DESC LIMIT 1
		`, s.dbName)
		fallbackArgs = []interface{}{data.SessionID}
	}
	fallbackResult := s.readWrite.QueryDb(fallbackQuery, fallbackArgs...)
	if fallbackResult.IsSuccess() {
		if rows, ok := fallbackResult.Data.([]map[string]interface{}); ok && len(rows) > 0 {
			newID := getIntValue(rows[0]["id"])
			if newID > 0 {
				log.Printf("✅ 成功创建训练数据（使用备用查询）: id=%d, session_id=%s", newID, data.SessionID)
				return newID, nil
			}
		}
	}

	return 0, fmt.Errorf("创建成功但无法获取数据ID")
}

// CreateTrainingDataBatch 批量创建训练数据
// 参数:
//   dataList: 训练数据列表
// 返回: 成功创建的数据ID列表和错误信息
func (s *SilicoidDataService) CreateTrainingDataBatch(dataList []TrainingData) ([]int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量创建训练数据异常: %v", r)
		}
	}()

	if len(dataList) == 0 {
		return nil, fmt.Errorf("训练数据列表不能为空")
	}

	ids := make([]int, 0, len(dataList))
	for _, data := range dataList {
		id, err := s.CreateTrainingData(data)
		if err != nil {
			return ids, fmt.Errorf("批量创建失败: %v", err)
		}
		ids = append(ids, id)
	}

	log.Printf("✅ 成功批量创建 %d 条训练数据", len(ids))
	return ids, nil
}

// ==================== 查询操作 ====================

// GetTrainingDataByID 根据ID获取训练数据
// 参数:
//   id: 数据ID
// 返回: 训练数据信息和错误信息
func (s *SilicoidDataService) GetTrainingDataByID(id int) (map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取训练数据异常: %v", r)
		}
	}()

	query := fmt.Sprintf(`
		SELECT id, session_id, conversations, system_prompt, is_positive, user_id,
		       feedback_reason, quality_score, tags, metadata, is_verified, is_enabled,
		       created_at, updated_at
		FROM %s.silicoid_training_dataset
		WHERE id = ?
	`, s.dbName)

	opResult := s.readWrite.QueryDb(query, id)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询训练数据失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return nil, fmt.Errorf("未找到 id=%d 的训练数据", id)
	}

	return rows[0], nil
}

// GetTrainingDataList 获取训练数据列表（支持过滤条件）
// 参数:
//   filters: 过滤条件
//     - session_id: 会话ID
//     - is_positive: 反馈类型（1=点赞, 0=点踩）
//     - user_id: 用户ID
//     - is_verified: 是否已验证（1=是, 0=否）
//     - is_enabled: 是否启用（1=启用, 0=禁用）
//     - min_quality_score: 最小质量评分
//   orderBy: 排序字段（默认 "created_at DESC"）
//   limit: 限制返回数量（默认 100）
//   offset: 偏移量（默认 0）
// 返回: 训练数据列表和错误信息
func (s *SilicoidDataService) GetTrainingDataList(
	filters map[string]interface{},
	orderBy string,
	limit int,
	offset int,
) ([]map[string]interface{}, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取训练数据列表异常: %v", r)
		}
	}()

	// 构建WHERE子句
	whereConditions := []string{}
	args := []interface{}{}

	if sessionID, ok := filters["session_id"].(string); ok && sessionID != "" {
		whereConditions = append(whereConditions, "session_id = ?")
		args = append(args, sessionID)
	}

	if isPositive, ok := filters["is_positive"]; ok {
		if ip, ok := isPositive.(int); ok {
			whereConditions = append(whereConditions, "is_positive = ?")
			args = append(args, ip)
		}
	}

	if userID, ok := filters["user_id"].(string); ok && userID != "" {
		whereConditions = append(whereConditions, "user_id = ?")
		args = append(args, userID)
	}

	if isVerified, ok := filters["is_verified"]; ok {
		if iv, ok := isVerified.(int); ok {
			whereConditions = append(whereConditions, "is_verified = ?")
			args = append(args, iv)
		}
	}

	if isEnabled, ok := filters["is_enabled"]; ok {
		if ie, ok := isEnabled.(int); ok {
			whereConditions = append(whereConditions, "is_enabled = ?")
			args = append(args, ie)
		}
	}

	if minQualityScore, ok := filters["min_quality_score"]; ok {
		if mqs, ok := minQualityScore.(int); ok {
			whereConditions = append(whereConditions, "quality_score >= ?")
			args = append(args, mqs)
		}
	}

	whereClause := ""
	if len(whereConditions) > 0 {
		whereClause = "WHERE " + strings.Join(whereConditions, " AND ")
	}

	// 设置默认排序
	if orderBy == "" {
		orderBy = "created_at DESC"
	}

	// 设置默认限制
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, session_id, conversations, system_prompt, is_positive, user_id,
		       feedback_reason, quality_score, tags, metadata, is_verified, is_enabled,
		       created_at, updated_at
		FROM %s.silicoid_training_dataset
		%s
		ORDER BY %s
		LIMIT ? OFFSET ?
	`, s.dbName, whereClause, orderBy)

	args = append(args, limit, offset)

	opResult := s.readWrite.QueryDb(query, args...)

	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询训练数据列表失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查询结果格式错误")
	}

	return rows, nil
}

// ==================== 更新操作 ====================

// UpdateTrainingData 更新训练数据
// 参数:
//   id: 数据ID
//   updates: 要更新的字段映射，可以包含以下字段：
//     - session_id, conversations, system_prompt, is_positive, user_id,
//       feedback_reason, quality_score, tags, metadata, is_verified, is_enabled
// 返回: 错误信息
func (s *SilicoidDataService) UpdateTrainingData(id int, updates map[string]interface{}) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("更新训练数据异常: %v", r)
		}
	}()

	if len(updates) == 0 {
		return fmt.Errorf("没有要更新的字段")
	}

	// 检查数据是否存在
	_, err := s.GetTrainingDataByID(id)
	if err != nil {
		return fmt.Errorf("训练数据不存在: %v", err)
	}

	// 允许更新的字段列表
	allowedFields := map[string]bool{
		"session_id":      true,
		"conversations":   true,
		"system_prompt":   true,
		"is_positive":     true,
		"user_id":         true,
		"feedback_reason": true,
		"quality_score":   true,
		"tags":            true,
		"metadata":        true,
		"is_verified":     true,
		"is_enabled":      true,
	}

	// 构建SET子句
	setClause := []string{}
	args := []interface{}{}

	for field, value := range updates {
		if !allowedFields[field] {
			continue // 跳过不允许的字段
		}

		// 验证JSON字段
		if field == "conversations" || field == "tags" || field == "metadata" {
			if strValue, ok := value.(string); ok {
				var jsonValue interface{}
				if err := json.Unmarshal([]byte(strValue), &jsonValue); err != nil {
					return fmt.Errorf("%s 必须是有效的JSON格式: %v", field, err)
				}
			}
		}

		// 特殊处理：如果字段值为 nil，设置为 NULL
		if value == nil {
			setClause = append(setClause, fmt.Sprintf("%s = NULL", field))
		} else {
			setClause = append(setClause, fmt.Sprintf("%s = ?", field))
			args = append(args, value)
		}
	}

	if len(setClause) == 0 {
		return fmt.Errorf("没有有效的更新字段")
	}

	// 添加 id 参数
	args = append(args, id)

	// 构建完整的 SET 子句
	setClauseStr := setClause[0]
	for i := 1; i < len(setClause); i++ {
		setClauseStr += ", " + setClause[i]
	}

	query := fmt.Sprintf(`
		UPDATE %s.silicoid_training_dataset
		SET %s
		WHERE id = ?
	`, s.dbName, setClauseStr)

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("更新训练数据失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功更新训练数据: id=%d", id)
	return nil
}

// TrainingDataUpdate 训练数据更新结构体
type TrainingDataUpdate struct {
	ID      int
	Updates map[string]interface{}
}

// UpdateTrainingDataBatch 批量更新训练数据
// 参数:
//   updates: 更新列表，每个元素包含 id 和要更新的字段
// 返回: 错误信息
func (s *SilicoidDataService) UpdateTrainingDataBatch(updates []TrainingDataUpdate) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量更新训练数据异常: %v", r)
		}
	}()

	if len(updates) == 0 {
		return fmt.Errorf("更新列表不能为空")
	}

	for _, update := range updates {
		if err := s.UpdateTrainingData(update.ID, update.Updates); err != nil {
			return fmt.Errorf("批量更新失败，id=%d: %v", update.ID, err)
		}
	}

	log.Printf("✅ 成功批量更新 %d 条训练数据", len(updates))
	return nil
}

// ==================== 删除操作 ====================

// DeleteTrainingData 软删除训练数据（设置is_enabled=0）
// 参数:
//   id: 数据ID
// 返回: 错误信息
func (s *SilicoidDataService) DeleteTrainingData(id int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("删除训练数据异常: %v", r)
		}
	}()

	// 检查数据是否存在
	_, err := s.GetTrainingDataByID(id)
	if err != nil {
		return fmt.Errorf("训练数据不存在: %v", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s.silicoid_training_dataset
		SET is_enabled = 0
		WHERE id = ?
	`, s.dbName)

	opResult := s.readWrite.ExecuteDb(query, id)
	if !opResult.IsSuccess() {
		return fmt.Errorf("删除训练数据失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功软删除训练数据: id=%d", id)
	return nil
}

// DeleteTrainingDataBatch 批量软删除训练数据
// 参数:
//   ids: 数据ID列表
// 返回: 错误信息
func (s *SilicoidDataService) DeleteTrainingDataBatch(ids []int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量删除训练数据异常: %v", r)
		}
	}()

	if len(ids) == 0 {
		return fmt.Errorf("数据ID列表不能为空")
	}

	// 构建批量更新语句
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		UPDATE %s.silicoid_training_dataset
		SET is_enabled = 0
		WHERE id IN (%s)
	`, s.dbName, strings.Join(placeholders, ", "))

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("批量软删除训练数据失败: %v", opResult.Error)
	}

	log.Printf("✅ 成功批量软删除 %d 条训练数据", len(ids))
	return nil
}

// HardDeleteTrainingData 硬删除训练数据（从数据库中物理删除）
// 警告：此操作不可恢复，请谨慎使用
// 参数:
//   id: 数据ID
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteTrainingData(id int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("硬删除训练数据异常: %v", r)
		}
	}()

	// 检查数据是否存在
	_, err := s.GetTrainingDataByID(id)
	if err != nil {
		return fmt.Errorf("训练数据不存在: %v", err)
	}

	query := fmt.Sprintf(`
		DELETE FROM %s.silicoid_training_dataset
		WHERE id = ?
	`, s.dbName)

	opResult := s.readWrite.ExecuteDb(query, id)
	if !opResult.IsSuccess() {
		return fmt.Errorf("硬删除训练数据失败: %v", opResult.Error)
	}

	log.Printf("⚠️ 成功硬删除训练数据: id=%d", id)
	return nil
}

// HardDeleteTrainingDataBatch 批量硬删除训练数据
// 警告：此操作不可恢复，请谨慎使用
// 参数:
//   ids: 数据ID列表
// 返回: 错误信息
func (s *SilicoidDataService) HardDeleteTrainingDataBatch(ids []int) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("批量硬删除训练数据异常: %v", r)
		}
	}()

	if len(ids) == 0 {
		return fmt.Errorf("数据ID列表不能为空")
	}

	// 构建批量删除语句
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		DELETE FROM %s.silicoid_training_dataset
		WHERE id IN (%s)
	`, s.dbName, strings.Join(placeholders, ", "))

	opResult := s.readWrite.ExecuteDb(query, args...)
	if !opResult.IsSuccess() {
		return fmt.Errorf("批量硬删除训练数据失败: %v", opResult.Error)
	}

	log.Printf("⚠️ 成功批量硬删除 %d 条训练数据", len(ids))
	return nil
}
