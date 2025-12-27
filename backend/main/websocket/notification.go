package websocket

import (
	"log"

	"digitalsingularity/backend/main/websocket/database"
)

var (
	notificationLogger *log.Logger
)

// InitNotificationService 初始化通知服务（统一接口）
func InitNotificationService(logger *log.Logger) {
	notificationLogger = logger
	InitOnlineNotification(logger)
	InitOfflineNotification(logger)
}

// SendNotification 发送通知（统一接口）
// 如果用户在线，则实时推送；如果用户离线，则保存到数据库
// 返回 true 表示用户在线并已发送，false 表示用户离线并已保存
func SendNotification(userID string, notificationType string, notificationData map[string]interface{}) bool {
	// 先尝试在线通知
	if SendOnlineNotification(userID, notificationType, notificationData) {
		// 用户在线，通知已发送
		return true
	}

	// 用户离线，保存到数据库
	if err := SaveOfflineNotification(userID, notificationType, notificationData); err != nil {
		if notificationLogger != nil {
			notificationLogger.Printf("[通知服务] 保存离线通知失败 (用户: %s, 类型: %s): %v", userID, notificationType, err)
		}
		return false
	}

	if notificationLogger != nil {
		notificationLogger.Printf("[通知服务] 用户离线，已保存通知 (用户: %s, 类型: %s)", userID, notificationType)
	}

	return false
}

// SendNotificationToMultipleUsers 向多个用户发送通知
// 返回在线用户数量和离线用户数量
func SendNotificationToMultipleUsers(userIDs []string, notificationType string, notificationData map[string]interface{}) (onlineCount, offlineCount int) {
	for _, userID := range userIDs {
		if SendNotification(userID, notificationType, notificationData) {
			onlineCount++
		} else {
			offlineCount++
		}
	}
	return
}

// GetUserUnreadNotifications 获取用户未读通知（包括在线时拉取离线通知）
func GetUserUnreadNotifications(userID string, limit int) ([]database.OfflineNotification, error) {
	return GetUnreadOfflineNotifications(userID, limit)
}

// MarkUserNotificationsAsRead 标记用户通知为已读
func MarkUserNotificationsAsRead(userID string, notificationIDs []int64) error {
	return MarkNotificationsAsRead(userID, notificationIDs)
}

// MarkAllUserNotificationsAsRead 标记用户所有通知为已读
func MarkAllUserNotificationsAsRead(userID string) error {
	return MarkAllNotificationsAsRead(userID)
}

// DeleteUserNotification 删除用户通知
func DeleteUserNotification(userID string, notificationID int64) error {
	return DeleteNotification(userID, notificationID)
}

// CheckUserOnlineStatus 检查用户是否在线（避免与 online_notification.go 中的函数名冲突）
func CheckUserOnlineStatus(userID string) bool {
	return IsUserOnline(userID)
}

