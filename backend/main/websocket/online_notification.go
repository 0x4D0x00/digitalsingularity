package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	onlineNotificationLogger *log.Logger
	userConnections          = make(map[string]map[*websocket.Conn]bool) // userID -> connections
	connectionUsers          = make(map[*websocket.Conn]string)         // conn -> userID
	onlineMutex              sync.RWMutex
)

// InitOnlineNotification 初始化在线通知服务
func InitOnlineNotification(logger *log.Logger) {
	onlineNotificationLogger = logger
}

// RegisterUserConnection 注册用户连接（用户上线时调用）
func RegisterUserConnection(userID string, conn *websocket.Conn) {
	onlineMutex.Lock()
	defer onlineMutex.Unlock()

	if userConnections[userID] == nil {
		userConnections[userID] = make(map[*websocket.Conn]bool)
	}
	userConnections[userID][conn] = true
	connectionUsers[conn] = userID

	if onlineNotificationLogger != nil {
		onlineNotificationLogger.Printf("[在线通知] 用户 %s 连接已注册，当前连接数: %d", userID, len(userConnections[userID]))
	}
}

// UnregisterUserConnection 注销用户连接（用户下线时调用）
func UnregisterUserConnection(conn *websocket.Conn) {
	onlineMutex.Lock()
	defer onlineMutex.Unlock()

	userID, exists := connectionUsers[conn]
	if !exists {
		return
	}

	delete(connectionUsers, conn)
	if userConnections[userID] != nil {
		delete(userConnections[userID], conn)
		if len(userConnections[userID]) == 0 {
			delete(userConnections, userID)
		}
	}

	if onlineNotificationLogger != nil {
		onlineNotificationLogger.Printf("[在线通知] 用户 %s 连接已注销", userID)
	}
}

// IsUserOnline 检查用户是否在线
func IsUserOnline(userID string) bool {
	onlineMutex.RLock()
	defer onlineMutex.RUnlock()

	conns, exists := userConnections[userID]
	return exists && len(conns) > 0
}

// SendOnlineNotification 向在线用户发送通知
// 如果用户在线，返回 true；如果用户离线，返回 false
func SendOnlineNotification(userID string, notificationType string, notificationData map[string]interface{}) bool {
	onlineMutex.RLock()
	conns, exists := userConnections[userID]
	if !exists || len(conns) == 0 {
		onlineMutex.RUnlock()
		return false
	}

	// 创建通知消息
	notification := map[string]interface{}{
		"type": notificationType,
		"data": notificationData,
	}

	notificationBytes, err := json.Marshal(notification)
	if err != nil {
		onlineMutex.RUnlock()
		if onlineNotificationLogger != nil {
			onlineNotificationLogger.Printf("[在线通知] 序列化通知失败: %v", err)
		}
		return false
	}

	// 复制连接列表，避免在发送时持有锁
	connList := make([]*websocket.Conn, 0, len(conns))
	for conn := range conns {
		connList = append(connList, conn)
	}
	onlineMutex.RUnlock()

	// 向所有连接发送通知
	successCount := 0
	for _, conn := range connList {
		if err := conn.WriteMessage(websocket.TextMessage, notificationBytes); err != nil {
			if onlineNotificationLogger != nil {
				onlineNotificationLogger.Printf("[在线通知] 发送通知失败 (用户: %s): %v", userID, err)
			}
			// 连接可能已断开，从映射中移除
			UnregisterUserConnection(conn)
		} else {
			successCount++
		}
	}

	if onlineNotificationLogger != nil && successCount > 0 {
		onlineNotificationLogger.Printf("[在线通知] 已向用户 %s 发送通知，成功: %d/%d", userID, successCount, len(connList))
	}

	return successCount > 0
}

// GetOnlineUserCount 获取在线用户数量
func GetOnlineUserCount() int {
	onlineMutex.RLock()
	defer onlineMutex.RUnlock()
	return len(userConnections)
}

// GetUserConnectionCount 获取指定用户的连接数
func GetUserConnectionCount(userID string) int {
	onlineMutex.RLock()
	defer onlineMutex.RUnlock()
	if conns, exists := userConnections[userID]; exists {
		return len(conns)
	}
	return 0
}

// GetUserIDByConnection 返回指定连接对应的 userID（如果存在）
func GetUserIDByConnection(conn *websocket.Conn) (string, bool) {
	onlineMutex.RLock()
	defer onlineMutex.RUnlock()
	userID, exists := connectionUsers[conn]
	return userID, exists
}

