package websocket

import (
	"log"

	"digitalsingularity/backend/main/websocket/database"
)

var (
	offlineNotificationLogger *log.Logger
)

// InitOfflineNotification 初始化离线通知服务
func InitOfflineNotification(logger *log.Logger) {
	offlineNotificationLogger = logger
}

// SaveOfflineNotification 保存离线通知
func SaveOfflineNotification(userID, notificationType string, notificationData map[string]interface{}) error {
	err := database.SaveOfflineNotificationToDB(userID, notificationType, notificationData)
	if err != nil {
		if offlineNotificationLogger != nil {
			offlineNotificationLogger.Printf("[离线通知] 保存通知失败 (用户: %s, 类型: %s): %v", userID, notificationType, err)
		}
		return err
	}

	if offlineNotificationLogger != nil {
		offlineNotificationLogger.Printf("[离线通知] 已保存通知 (用户: %s, 类型: %s)", userID, notificationType)
	}

	return nil
}

// GetUnreadOfflineNotifications 获取用户的未读离线通知
func GetUnreadOfflineNotifications(userID string, limit int) ([]database.OfflineNotification, error) {
	notifications, err := database.GetUnreadOfflineNotificationsFromDB(userID, limit)
	if err != nil {
		if offlineNotificationLogger != nil {
			offlineNotificationLogger.Printf("[离线通知] 获取未读通知失败 (用户: %s): %v", userID, err)
		}
		return nil, err
	}

	if offlineNotificationLogger != nil {
		offlineNotificationLogger.Printf("[离线通知] 获取未读通知 (用户: %s, 数量: %d)", userID, len(notifications))
	}

	return notifications, nil
}

// MarkNotificationsAsRead 标记通知为已读
func MarkNotificationsAsRead(userID string, notificationIDs []int64) error {
	err := database.MarkNotificationsAsReadInDB(userID, notificationIDs)
	if err != nil {
		if offlineNotificationLogger != nil {
			offlineNotificationLogger.Printf("[离线通知] 标记通知为已读失败 (用户: %s): %v", userID, err)
		}
		return err
	}

	if offlineNotificationLogger != nil {
		offlineNotificationLogger.Printf("[离线通知] 已标记通知为已读 (用户: %s, 数量: %d)", userID, len(notificationIDs))
	}

	return nil
}

// MarkAllNotificationsAsRead 标记用户所有未读通知为已读
func MarkAllNotificationsAsRead(userID string) error {
	err := database.MarkAllNotificationsAsReadInDB(userID)
	if err != nil {
		if offlineNotificationLogger != nil {
			offlineNotificationLogger.Printf("[离线通知] 标记所有通知为已读失败 (用户: %s): %v", userID, err)
		}
		return err
	}

	if offlineNotificationLogger != nil {
		offlineNotificationLogger.Printf("[离线通知] 已标记所有通知为已读 (用户: %s)", userID)
	}

	return nil
}

// DeleteNotification 删除通知
func DeleteNotification(userID string, notificationID int64) error {
	err := database.DeleteNotificationInDB(userID, notificationID)
	if err != nil {
		if offlineNotificationLogger != nil {
			offlineNotificationLogger.Printf("[离线通知] 删除通知失败 (用户: %s, 通知ID: %d): %v", userID, notificationID, err)
		}
		return err
	}

	if offlineNotificationLogger != nil {
		offlineNotificationLogger.Printf("[离线通知] 已删除通知 (用户: %s, 通知ID: %d)", userID, notificationID)
	}

	return nil
}

