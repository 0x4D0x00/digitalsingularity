package trainingdata

import (
	"encoding/json"
	"fmt"
	"strconv"

	"digitalsingularity/backend/silicoid/database"
)

// TrainingDataService 训练数据服务
type TrainingDataService struct {
	dbService *database.SilicoidDataService
}

// NewTrainingDataService 创建训练数据服务实例
func NewTrainingDataService(dbService *database.SilicoidDataService) *TrainingDataService {
	return &TrainingDataService{
		dbService: dbService,
	}
}

// HandleCreate 处理创建训练数据请求
func (s *TrainingDataService) HandleCreate(data map[string]interface{}) map[string]interface{} {
	// 构建TrainingData结构
	trainingData := database.TrainingData{}

	// 解析必填字段
	if sessionID, ok := data["session_id"].(string); ok {
		trainingData.SessionID = sessionID
	} else {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: session_id",
		}
	}

	if conversations, ok := data["conversations"].(string); ok {
		trainingData.Conversations = conversations
	} else {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: conversations",
		}
	}

	// 解析可选字段
	if systemPrompt, ok := data["system_prompt"].(string); ok && systemPrompt != "" {
		trainingData.SystemPrompt = &systemPrompt
	}

	if isPositive, ok := data["is_positive"]; ok {
		if ip, ok := isPositive.(float64); ok {
			trainingData.IsPositive = int(ip)
		} else if ip, ok := isPositive.(int); ok {
			trainingData.IsPositive = ip
		}
	}

	if userID, ok := data["user_id"].(string); ok && userID != "" {
		trainingData.UserID = &userID
	}

	if feedbackReason, ok := data["feedback_reason"].(string); ok && feedbackReason != "" {
		trainingData.FeedbackReason = &feedbackReason
	}

	if qualityScore, ok := data["quality_score"]; ok {
		if qs, ok := qualityScore.(float64); ok {
			qsInt := int(qs)
			trainingData.QualityScore = &qsInt
		} else if qs, ok := qualityScore.(int); ok {
			trainingData.QualityScore = &qs
		}
	}

	if tags, ok := data["tags"].(string); ok && tags != "" {
		trainingData.Tags = &tags
	} else if tagsObj, ok := data["tags"]; ok {
		// 如果是对象，转换为JSON字符串
		if tagsJSON, err := json.Marshal(tagsObj); err == nil {
			tagsStr := string(tagsJSON)
			trainingData.Tags = &tagsStr
		}
	}

	if metadata, ok := data["metadata"].(string); ok && metadata != "" {
		trainingData.Metadata = &metadata
	} else if metadataObj, ok := data["metadata"]; ok {
		// 如果是对象，转换为JSON字符串
		if metadataJSON, err := json.Marshal(metadataObj); err == nil {
			metadataStr := string(metadataJSON)
			trainingData.Metadata = &metadataStr
		}
	}

	if isVerified, ok := data["is_verified"]; ok {
		if iv, ok := isVerified.(float64); ok {
			ivInt := int(iv)
			trainingData.IsVerified = &ivInt
		} else if iv, ok := isVerified.(int); ok {
			trainingData.IsVerified = &iv
		}
	}

	if isEnabled, ok := data["is_enabled"]; ok {
		if ie, ok := isEnabled.(float64); ok {
			ieInt := int(ie)
			trainingData.IsEnabled = &ieInt
		} else if ie, ok := isEnabled.(int); ok {
			trainingData.IsEnabled = &ie
		}
	}

	// 调用数据库层创建
	id, err := s.dbService.CreateTrainingData(trainingData)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("创建训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"id": id,
		},
	}
}

// HandleCreateBatch 处理批量创建训练数据请求
func (s *TrainingDataService) HandleCreateBatch(data map[string]interface{}) map[string]interface{} {
	dataList, ok := data["data_list"].([]interface{})
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: data_list",
		}
	}

	if len(dataList) == 0 {
		return map[string]interface{}{
			"status":  "fail",
			"message": "data_list 不能为空",
		}
	}

	// 转换数据列表
	trainingDataList := make([]database.TrainingData, 0, len(dataList))
	for i, item := range dataList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("data_list[%d] 格式错误", i),
			}
		}

		trainingData := database.TrainingData{}

		// 解析必填字段
		if sessionID, ok := itemMap["session_id"].(string); ok {
			trainingData.SessionID = sessionID
		} else {
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("data_list[%d] 缺少必填字段: session_id", i),
			}
		}

		if conversations, ok := itemMap["conversations"].(string); ok {
			trainingData.Conversations = conversations
		} else {
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("data_list[%d] 缺少必填字段: conversations", i),
			}
		}

		// 解析可选字段（与HandleCreate相同的逻辑）
		if systemPrompt, ok := itemMap["system_prompt"].(string); ok && systemPrompt != "" {
			trainingData.SystemPrompt = &systemPrompt
		}

		if isPositive, ok := itemMap["is_positive"]; ok {
			if ip, ok := isPositive.(float64); ok {
				trainingData.IsPositive = int(ip)
			} else if ip, ok := isPositive.(int); ok {
				trainingData.IsPositive = ip
			}
		}

		if userID, ok := itemMap["user_id"].(string); ok && userID != "" {
			trainingData.UserID = &userID
		}

		if feedbackReason, ok := itemMap["feedback_reason"].(string); ok && feedbackReason != "" {
			trainingData.FeedbackReason = &feedbackReason
		}

		if qualityScore, ok := itemMap["quality_score"]; ok {
			if qs, ok := qualityScore.(float64); ok {
				qsInt := int(qs)
				trainingData.QualityScore = &qsInt
			} else if qs, ok := qualityScore.(int); ok {
				trainingData.QualityScore = &qs
			}
		}

		if tags, ok := itemMap["tags"].(string); ok && tags != "" {
			trainingData.Tags = &tags
		} else if tagsObj, ok := itemMap["tags"]; ok {
			if tagsJSON, err := json.Marshal(tagsObj); err == nil {
				tagsStr := string(tagsJSON)
				trainingData.Tags = &tagsStr
			}
		}

		if metadata, ok := itemMap["metadata"].(string); ok && metadata != "" {
			trainingData.Metadata = &metadata
		} else if metadataObj, ok := itemMap["metadata"]; ok {
			if metadataJSON, err := json.Marshal(metadataObj); err == nil {
				metadataStr := string(metadataJSON)
				trainingData.Metadata = &metadataStr
			}
		}

		if isVerified, ok := itemMap["is_verified"]; ok {
			if iv, ok := isVerified.(float64); ok {
				ivInt := int(iv)
				trainingData.IsVerified = &ivInt
			} else if iv, ok := isVerified.(int); ok {
				trainingData.IsVerified = &iv
			}
		}

		if isEnabled, ok := itemMap["is_enabled"]; ok {
			if ie, ok := isEnabled.(float64); ok {
				ieInt := int(ie)
				trainingData.IsEnabled = &ieInt
			} else if ie, ok := isEnabled.(int); ok {
				trainingData.IsEnabled = &ie
			}
		}

		trainingDataList = append(trainingDataList, trainingData)
	}

	// 调用数据库层批量创建
	ids, err := s.dbService.CreateTrainingDataBatch(trainingDataList)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("批量创建训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"ids": ids,
		},
	}
}

// HandleGet 处理获取单个训练数据请求
func (s *TrainingDataService) HandleGet(data map[string]interface{}) map[string]interface{} {
	id, ok := data["id"]
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: id",
		}
	}

	var idInt int
	switch v := id.(type) {
	case float64:
		idInt = int(v)
	case int:
		idInt = v
	case string:
		var err error
		idInt, err = strconv.Atoi(v)
		if err != nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "id 格式错误",
			}
		}
	default:
		return map[string]interface{}{
			"status":  "fail",
			"message": "id 格式错误",
		}
	}

	// 调用数据库层查询
	result, err := s.dbService.GetTrainingDataByID(idInt)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data":   result,
	}
}

// HandleList 处理获取训练数据列表请求
func (s *TrainingDataService) HandleList(data map[string]interface{}) map[string]interface{} {
	// 构建过滤条件
	filters := make(map[string]interface{})

	if sessionID, ok := data["session_id"].(string); ok && sessionID != "" {
		filters["session_id"] = sessionID
	}

	if isPositive, ok := data["is_positive"]; ok {
		if ip, ok := isPositive.(float64); ok {
			filters["is_positive"] = int(ip)
		} else if ip, ok := isPositive.(int); ok {
			filters["is_positive"] = ip
		}
	}

	if userID, ok := data["user_id"].(string); ok && userID != "" {
		filters["user_id"] = userID
	}

	if isVerified, ok := data["is_verified"]; ok {
		if iv, ok := isVerified.(float64); ok {
			filters["is_verified"] = int(iv)
		} else if iv, ok := isVerified.(int); ok {
			filters["is_verified"] = iv
		}
	}

	if isEnabled, ok := data["is_enabled"]; ok {
		if ie, ok := isEnabled.(float64); ok {
			filters["is_enabled"] = int(ie)
		} else if ie, ok := isEnabled.(int); ok {
			filters["is_enabled"] = ie
		}
	}

	if minQualityScore, ok := data["min_quality_score"]; ok {
		if mqs, ok := minQualityScore.(float64); ok {
			filters["min_quality_score"] = int(mqs)
		} else if mqs, ok := minQualityScore.(int); ok {
			filters["min_quality_score"] = mqs
		}
	}

	// 获取排序字段
	orderBy := "created_at DESC"
	if ob, ok := data["order_by"].(string); ok && ob != "" {
		orderBy = ob
	}

	// 获取分页参数
	limit := 100
	if l, ok := data["limit"]; ok {
		if limitVal, ok := l.(float64); ok {
			limit = int(limitVal)
		} else if limitVal, ok := l.(int); ok {
			limit = limitVal
		}
	}

	offset := 0
	if o, ok := data["offset"]; ok {
		if offsetVal, ok := o.(float64); ok {
			offset = int(offsetVal)
		} else if offsetVal, ok := o.(int); ok {
			offset = offsetVal
		}
	}

	// 调用数据库层查询列表
	results, err := s.dbService.GetTrainingDataList(filters, orderBy, limit, offset)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("获取训练数据列表失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"list":  results,
			"count": len(results),
		},
	}
}

// HandleUpdate 处理更新训练数据请求
func (s *TrainingDataService) HandleUpdate(data map[string]interface{}) map[string]interface{} {
	id, ok := data["id"]
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: id",
		}
	}

	var idInt int
	switch v := id.(type) {
	case float64:
		idInt = int(v)
	case int:
		idInt = v
	case string:
		var err error
		idInt, err = strconv.Atoi(v)
		if err != nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "id 格式错误",
			}
		}
	default:
		return map[string]interface{}{
			"status":  "fail",
			"message": "id 格式错误",
		}
	}

	// 构建更新字段
	updates := make(map[string]interface{})

	// 允许更新的字段
	allowedFields := []string{
		"session_id", "conversations", "system_prompt", "is_positive",
		"user_id", "feedback_reason", "quality_score", "tags",
		"metadata", "is_verified", "is_enabled",
	}

	for _, field := range allowedFields {
		if value, ok := data[field]; ok {
			// 处理JSON字符串字段
			if field == "tags" || field == "metadata" {
				if strVal, ok := value.(string); ok {
					updates[field] = strVal
				} else {
					// 如果是对象，转换为JSON字符串
					if jsonBytes, err := json.Marshal(value); err == nil {
						updates[field] = string(jsonBytes)
					}
				}
			} else {
				updates[field] = value
			}
		}
	}

	if len(updates) == 0 {
		return map[string]interface{}{
			"status":  "fail",
			"message": "没有要更新的字段",
		}
	}

	// 调用数据库层更新
	err := s.dbService.UpdateTrainingData(idInt, updates)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("更新训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"id": idInt,
		},
	}
}

// HandleUpdateBatch 处理批量更新训练数据请求
func (s *TrainingDataService) HandleUpdateBatch(data map[string]interface{}) map[string]interface{} {
	updatesList, ok := data["updates_list"].([]interface{})
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: updates_list",
		}
	}

	if len(updatesList) == 0 {
		return map[string]interface{}{
			"status":  "fail",
			"message": "updates_list 不能为空",
		}
	}

	// 转换更新列表
	trainingUpdates := make([]database.TrainingDataUpdate, 0, len(updatesList))
	for i, item := range updatesList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("updates_list[%d] 格式错误", i),
			}
		}

		update := database.TrainingDataUpdate{}

		// 解析ID
		if id, ok := itemMap["id"]; ok {
			switch v := id.(type) {
			case float64:
				update.ID = int(v)
			case int:
				update.ID = v
			case string:
				idInt, err := strconv.Atoi(v)
				if err != nil {
					return map[string]interface{}{
						"status":  "fail",
						"message": fmt.Sprintf("updates_list[%d].id 格式错误", i),
					}
				}
				update.ID = idInt
			default:
				return map[string]interface{}{
					"status":  "fail",
					"message": fmt.Sprintf("updates_list[%d].id 格式错误", i),
				}
			}
		} else {
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("updates_list[%d] 缺少必填字段: id", i),
			}
		}

		// 解析更新字段
		updates := make(map[string]interface{})
		allowedFields := []string{
			"session_id", "conversations", "system_prompt", "is_positive",
			"user_id", "feedback_reason", "quality_score", "tags",
			"metadata", "is_verified", "is_enabled",
		}

		for _, field := range allowedFields {
			if value, ok := itemMap[field]; ok {
				if field == "tags" || field == "metadata" {
					if strVal, ok := value.(string); ok {
						updates[field] = strVal
					} else {
						if jsonBytes, err := json.Marshal(value); err == nil {
							updates[field] = string(jsonBytes)
						}
					}
				} else {
					updates[field] = value
				}
			}
		}

		update.Updates = updates
		trainingUpdates = append(trainingUpdates, update)
	}

	// 调用数据库层批量更新
	err := s.dbService.UpdateTrainingDataBatch(trainingUpdates)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("批量更新训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"count": len(trainingUpdates),
		},
	}
}

// HandleDelete 处理删除训练数据请求（软删除）
func (s *TrainingDataService) HandleDelete(data map[string]interface{}) map[string]interface{} {
	id, ok := data["id"]
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: id",
		}
	}

	var idInt int
	switch v := id.(type) {
	case float64:
		idInt = int(v)
	case int:
		idInt = v
	case string:
		var err error
		idInt, err = strconv.Atoi(v)
		if err != nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "id 格式错误",
			}
		}
	default:
		return map[string]interface{}{
			"status":  "fail",
			"message": "id 格式错误",
		}
	}

	// 调用数据库层删除
	err := s.dbService.DeleteTrainingData(idInt)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("删除训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"id": idInt,
		},
	}
}

// HandleDeleteBatch 处理批量删除训练数据请求（软删除）
func (s *TrainingDataService) HandleDeleteBatch(data map[string]interface{}) map[string]interface{} {
	ids, ok := data["ids"].([]interface{})
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: ids",
		}
	}

	if len(ids) == 0 {
		return map[string]interface{}{
			"status":  "fail",
			"message": "ids 不能为空",
		}
	}

	// 转换ID列表
	idList := make([]int, 0, len(ids))
	for i, id := range ids {
		var idInt int
		switch v := id.(type) {
		case float64:
			idInt = int(v)
		case int:
			idInt = v
		case string:
			var err error
			idInt, err = strconv.Atoi(v)
			if err != nil {
				return map[string]interface{}{
					"status":  "fail",
					"message": fmt.Sprintf("ids[%d] 格式错误", i),
				}
			}
		default:
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("ids[%d] 格式错误", i),
			}
		}
		idList = append(idList, idInt)
	}

	// 调用数据库层批量删除
	err := s.dbService.DeleteTrainingDataBatch(idList)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("批量删除训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"count": len(idList),
		},
	}
}

// HandleHardDelete 处理硬删除训练数据请求
func (s *TrainingDataService) HandleHardDelete(data map[string]interface{}) map[string]interface{} {
	id, ok := data["id"]
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: id",
		}
	}

	var idInt int
	switch v := id.(type) {
	case float64:
		idInt = int(v)
	case int:
		idInt = v
	case string:
		var err error
		idInt, err = strconv.Atoi(v)
		if err != nil {
			return map[string]interface{}{
				"status":  "fail",
				"message": "id 格式错误",
			}
		}
	default:
		return map[string]interface{}{
			"status":  "fail",
			"message": "id 格式错误",
		}
	}

	// 调用数据库层硬删除
	err := s.dbService.HardDeleteTrainingData(idInt)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("硬删除训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"id": idInt,
		},
	}
}

// HandleHardDeleteBatch 处理批量硬删除训练数据请求
func (s *TrainingDataService) HandleHardDeleteBatch(data map[string]interface{}) map[string]interface{} {
	ids, ok := data["ids"].([]interface{})
	if !ok {
		return map[string]interface{}{
			"status":  "fail",
			"message": "缺少必填字段: ids",
		}
	}

	if len(ids) == 0 {
		return map[string]interface{}{
			"status":  "fail",
			"message": "ids 不能为空",
		}
	}

	// 转换ID列表
	idList := make([]int, 0, len(ids))
	for i, id := range ids {
		var idInt int
		switch v := id.(type) {
		case float64:
			idInt = int(v)
		case int:
			idInt = v
		case string:
			var err error
			idInt, err = strconv.Atoi(v)
			if err != nil {
				return map[string]interface{}{
					"status":  "fail",
					"message": fmt.Sprintf("ids[%d] 格式错误", i),
				}
			}
		default:
			return map[string]interface{}{
				"status":  "fail",
				"message": fmt.Sprintf("ids[%d] 格式错误", i),
			}
		}
		idList = append(idList, idInt)
	}

	// 调用数据库层批量硬删除
	err := s.dbService.HardDeleteTrainingDataBatch(idList)
	if err != nil {
		return map[string]interface{}{
			"status":  "fail",
			"message": fmt.Sprintf("批量硬删除训练数据失败: %v", err),
		}
	}

	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"count": len(idList),
		},
	}
}

