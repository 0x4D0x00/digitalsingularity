package database

import (
	"encoding/json"
	"fmt"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
)

// OfflineNotification 离线通知结构
type OfflineNotification struct {
	ID               int64                  `json:"id"`
	UserID           string                 `json:"user_id"`
	NotificationType string                 `json:"notification_type"`
	NotificationData map[string]interface{} `json:"notification_data"`
	CreatedAt        time.Time              `json:"created_at"`
	ReadAt           *time.Time             `json:"read_at,omitempty"`
	DeletedAt        *time.Time             `json:"deleted_at,omitempty"`
}

// getNotificationService 获取通知数据库服务实例
func getNotificationService() (*datahandle.CommonReadWriteService, error) {
	return datahandle.NewCommonReadWriteService("common")
}

// SaveOfflineNotificationToDB 保存离线通知到数据库
func SaveOfflineNotificationToDB(userID, notificationType string, notificationData map[string]interface{}) error {
	service, err := getNotificationService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	// 将 notificationData 转换为 JSON 字符串
	notificationDataJSON, err := json.Marshal(notificationData)
	if err != nil {
		return fmt.Errorf("序列化通知数据失败: %v", err)
	}

	insertQuery := `INSERT INTO offline_notifications (user_id, notification_type, notification_data, created_at) 
		VALUES (?, ?, ?, NOW())`
	
	insertResult := service.ExecuteDb(insertQuery, userID, notificationType, string(notificationDataJSON))
	if !insertResult.IsSuccess() {
		return fmt.Errorf("保存离线通知失败: %v", insertResult.Error)
	}

	return nil
}

// GetUnreadOfflineNotificationsFromDB 从数据库获取用户的未读离线通知
func GetUnreadOfflineNotificationsFromDB(userID string, limit int) ([]OfflineNotification, error) {
	service, err := getNotificationService()
	if err != nil {
		return nil, fmt.Errorf("获取数据库服务失败: %v", err)
	}

	if limit <= 0 {
		limit = 100 // 默认限制100条
	}

	query := `SELECT id, user_id, notification_type, notification_data, created_at, read_at, deleted_at
		FROM offline_notifications
		WHERE user_id = ? AND read_at IS NULL AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ?`

	queryResult := service.QueryDb(query, userID, limit)
	if !queryResult.IsSuccess() {
		return nil, fmt.Errorf("查询离线通知失败: %v", queryResult.Error)
	}

	notifications := make([]OfflineNotification, 0)
	rows, ok := queryResult.Data.([]map[string]interface{})
	if !ok {
		return notifications, nil
	}

	for _, row := range rows {
		notification := OfflineNotification{}
		
		if id, ok := row["id"].(int64); ok {
			notification.ID = id
		}
		if uid, ok := row["user_id"].(string); ok {
			notification.UserID = uid
		}
		if ntype, ok := row["notification_type"].(string); ok {
			notification.NotificationType = ntype
		}
		if ndata, ok := row["notification_data"].(string); ok {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(ndata), &data); err == nil {
				notification.NotificationData = data
			}
		}
		if createdAt, ok := row["created_at"].(time.Time); ok {
			notification.CreatedAt = createdAt
		} else if createdAtStr, ok := row["created_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
				notification.CreatedAt = t
			}
		}
		if readAt, ok := row["read_at"].(time.Time); ok {
			notification.ReadAt = &readAt
		} else if readAtStr, ok := row["read_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", readAtStr); err == nil {
				notification.ReadAt = &t
			}
		}
		if deletedAt, ok := row["deleted_at"].(time.Time); ok {
			notification.DeletedAt = &deletedAt
		} else if deletedAtStr, ok := row["deleted_at"].(string); ok {
			if t, err := time.Parse("2006-01-02 15:04:05", deletedAtStr); err == nil {
				notification.DeletedAt = &t
			}
		}

		notifications = append(notifications, notification)
	}

	return notifications, nil
}

// MarkNotificationsAsReadInDB 在数据库中标记通知为已读
func MarkNotificationsAsReadInDB(userID string, notificationIDs []int64) error {
	service, err := getNotificationService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	if len(notificationIDs) == 0 {
		return nil
	}

	// 构建 SQL IN 子句
	args := make([]interface{}, len(notificationIDs)+1)
	args[0] = userID
	updateQuery := `UPDATE offline_notifications 
		SET read_at = NOW() 
		WHERE user_id = ? AND id IN (`
	for i, id := range notificationIDs {
		if i > 0 {
			updateQuery += ","
		}
		updateQuery += "?"
		args[i+1] = id
	}
	updateQuery += ") AND read_at IS NULL"

	updateResult := service.ExecuteDb(updateQuery, args...)
	if !updateResult.IsSuccess() {
		return fmt.Errorf("标记通知为已读失败: %v", updateResult.Error)
	}

	return nil
}

// MarkAllNotificationsAsReadInDB 在数据库中标记用户所有未读通知为已读
func MarkAllNotificationsAsReadInDB(userID string) error {
	service, err := getNotificationService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	updateQuery := `UPDATE offline_notifications 
		SET read_at = NOW() 
		WHERE user_id = ? AND read_at IS NULL AND deleted_at IS NULL`

	updateResult := service.ExecuteDb(updateQuery, userID)
	if !updateResult.IsSuccess() {
		return fmt.Errorf("标记所有通知为已读失败: %v", updateResult.Error)
	}

	return nil
}

// DeleteNotificationInDB 在数据库中软删除通知
func DeleteNotificationInDB(userID string, notificationID int64) error {
	service, err := getNotificationService()
	if err != nil {
		return fmt.Errorf("获取数据库服务失败: %v", err)
	}

	updateQuery := `UPDATE offline_notifications 
		SET deleted_at = NOW() 
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`

	updateResult := service.ExecuteDb(updateQuery, notificationID, userID)
	if !updateResult.IsSuccess() {
		return fmt.Errorf("删除通知失败: %v", updateResult.Error)
	}

	return nil
}

