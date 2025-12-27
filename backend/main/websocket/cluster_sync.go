package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	clusterRedisClient *redis.Client
	clusterLogger      *log.Logger
	ctx                = context.Background()
	pubsubChannels     = "websocket:notifications"
	serverID           string // 当前服务器的唯一标识
)

// ClusterNotificationMessage 集群通知消息
type ClusterNotificationMessage struct {
	ServerID         string                 `json:"server_id"`         // 发送服务器ID
	UserID           string                 `json:"user_id"`           // 目标用户ID
	ContactIDs       []string               `json:"contact_ids"`       // 联系人ID列表
	NotificationType string                 `json:"notification_type"` // 通知类型
	Payload          map[string]interface{} `json:"payload"`           // 通知数据
	Timestamp        time.Time              `json:"timestamp"`         // 时间戳
}

// InitClusterSync 初始化集群同步（使用现有的 Redis 客户端）
func InitClusterSync(existingRedisClient *redis.Client, currentServerID string, logger *log.Logger) error {
	clusterLogger = logger
	serverID = currentServerID
	clusterRedisClient = existingRedisClient

	if clusterRedisClient == nil {
		return fmt.Errorf("Redis 客户端为空")
	}

	// 测试连接
	if err := clusterRedisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis 连接测试失败: %v", err)
	}

	if clusterLogger != nil {
		clusterLogger.Printf("[集群同步] 使用现有 Redis 连接，服务器ID: %s", serverID)
	}

	// 启动订阅监听
	go subscribeClusterNotifications()

	return nil
}

// PublishClusterNotification 发布集群通知（通知其他服务器）
func PublishClusterNotification(userID, notificationType string, contactIDs []string, payload map[string]interface{}) error {
	if clusterRedisClient == nil {
		// Redis 未初始化，跳过集群同步
		return nil
	}

	message := ClusterNotificationMessage{
		ServerID:         serverID,
		UserID:           userID,
		ContactIDs:       contactIDs,
		NotificationType: notificationType,
		Payload:          payload,
		Timestamp:        time.Now().UTC(),
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化集群消息失败: %v", err)
	}

	// 发布到 Redis 频道
	if err := clusterRedisClient.Publish(ctx, pubsubChannels, messageBytes).Err(); err != nil {
		return fmt.Errorf("发布集群消息失败: %v", err)
	}

	if clusterLogger != nil {
		clusterLogger.Printf("[集群同步] 已发布通知到集群 (类型: %s, 用户: %s, 联系人数: %d)",
			notificationType, userID, len(contactIDs))
	}

	return nil
}

// subscribeClusterNotifications 订阅集群通知
func subscribeClusterNotifications() {
	pubsub := clusterRedisClient.Subscribe(ctx, pubsubChannels)
	defer pubsub.Close()

	if clusterLogger != nil {
		clusterLogger.Printf("[集群同步] 开始监听集群通知频道: %s", pubsubChannels)
	}

	ch := pubsub.Channel()
	for msg := range ch {
		handleClusterNotification(msg.Payload)
	}
}

// handleClusterNotification 处理接收到的集群通知
func handleClusterNotification(payload string) {
	var message ClusterNotificationMessage
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		if clusterLogger != nil {
			clusterLogger.Printf("[集群同步] 解析集群消息失败: %v", err)
		}
		return
	}

	// 忽略自己发送的消息
	if message.ServerID == serverID {
		return
	}

	if clusterLogger != nil {
		clusterLogger.Printf("[集群同步] 收到来自服务器 %s 的通知 (类型: %s, 用户: %s)",
			message.ServerID, message.NotificationType, message.UserID)
	}

	// 向本地连接的用户发送通知
	notificationPayload := message.Payload
	if notificationPayload == nil {
		notificationPayload = make(map[string]interface{})
	}

	// 通知目标用户
	SendNotification(message.UserID, message.NotificationType, notificationPayload)

	// 通知联系人
	if len(message.ContactIDs) > 0 {
		SendNotificationToMultipleUsers(message.ContactIDs, message.NotificationType, notificationPayload)
	}
}

// CloseClusterSync 关闭集群同步（不关闭 Redis 连接，因为是共享的）
func CloseClusterSync() {
	if clusterLogger != nil {
		clusterLogger.Printf("[集群同步] 停止集群同步服务")
	}
	// 注意：不关闭 Redis 客户端，因为它是共享的
}
