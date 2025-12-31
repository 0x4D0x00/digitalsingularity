package relationship_management

import (
	"digitalsingularity/backend/communicationsystem/database"
)

// FriendWithUserInfo 关系管理层暴露的好友+用户信息结构
type FriendWithUserInfo = database.FriendWithUserInfo

// AddFriend 添加好友请求（接受 communication_id）
func AddFriend(userCommunicationID, friendCommunicationID, remark string) error {
	return database.AddFriend(userCommunicationID, friendCommunicationID, remark)
}

// AcceptFriend 接受好友请求（接受 communication_id）
func AcceptFriend(userCommunicationID, friendCommunicationID, remark string) error {
	return database.AcceptFriend(userCommunicationID, friendCommunicationID, remark)
}

// RejectFriend 拒绝好友请求（接受 communication_id）
func RejectFriend(userCommunicationID, friendCommunicationID string) error {
	return database.RejectFriend(userCommunicationID, friendCommunicationID)
}

// CheckFriend 检查好友关系（接受 communication_id）
func CheckFriend(userCommunicationID, friendCommunicationID string) (bool, error) {
	return database.CheckFriend(userCommunicationID, friendCommunicationID)
}

// ImportFriends 批量导入好友（接受 communication_id）
func ImportFriends(userCommunicationID string, friendCommunicationIDs []string) error {
	return database.ImportFriends(userCommunicationID, friendCommunicationIDs)
}

// GetFriends 获取好友列表（接受 communication_id）
func GetFriends(userCommunicationID string, status string) ([]database.FriendWithUserInfo, error) {
	return database.GetFriends(userCommunicationID, status)
}

// GetFriendRequests 获取收到的好友请求（接受 communication_id）
func GetFriendRequests(userCommunicationID string) ([]database.FriendWithUserInfo, error) {
	return database.GetFriendRequests(userCommunicationID)
}

// SearchUser 搜索用户（接受 communication_id）
func SearchUser(userCommunicationID, keyword string) ([]database.FriendWithUserInfo, error) {
	return database.SearchUser(userCommunicationID, keyword)
}

// RemoveFriend 删除好友（接受 communication_id）
func RemoveFriend(userCommunicationID, friendCommunicationID string) error {
	return database.RemoveFriend(userCommunicationID, friendCommunicationID)
}

// GetFriendStats 获取好友数量（接受 communication_id）
func GetFriendStats(userCommunicationID string) (int, error) {
	return database.GetFriendStats(userCommunicationID)
}

// UpdateFriend 更新好友备注（接受 communication_id）
func UpdateFriend(userCommunicationID, friendCommunicationID, remark string) error {
	return database.UpdateFriend(userCommunicationID, friendCommunicationID, remark)
}

// FollowUser 关注用户（接受 communication_id）
func FollowUser(followerCommunicationID, followingCommunicationID string) error {
	return database.FollowUser(followerCommunicationID, followingCommunicationID)
}

// CheckFollow 检查关注关系（接受 communication_id）
func CheckFollow(followerCommunicationID, followingCommunicationID string) (bool, error) {
	return database.CheckFollow(followerCommunicationID, followingCommunicationID)
}

// GetFollowing 获取关注列表（接受 communication_id）
func GetFollowing(userCommunicationID string) ([]database.User, error) {
	return database.GetFollowing(userCommunicationID)
}

// GetMutualFollows 获取互关列表（接受 communication_id）
func GetMutualFollows(userCommunicationID string) ([]database.User, error) {
	return database.GetMutualFollows(userCommunicationID)
}

// UnfollowUser 取消关注（接受 communication_id）
func UnfollowUser(followerCommunicationID, followingCommunicationID string) error {
	return database.UnfollowUser(followerCommunicationID, followingCommunicationID)
}

// GetFollowStats 获取关注统计（接受 communication_id）
func GetFollowStats(userCommunicationID string) (database.FollowStats, error) {
	return database.GetFollowStats(userCommunicationID)
}

// GetFollowers 获取粉丝列表（接受 communication_id）
func GetFollowers(userCommunicationID string) ([]database.User, error) {
	return database.GetFollowers(userCommunicationID)
}

// GetCommunicationIDByUserID 根据内部 user_id 获取通信系统 ID
// 注意：此函数主要用于 http 转换userCommunicationID场景，业务逻辑应使用 user_id
func GetCommunicationIDByUserID(userID string) (string, error) {
	service, err := database.GetServiceForRelationship()
	if err != nil {
		return "", err
	}
	return database.GetCommunicationIDByUserID(service, userID)
}

// GetUserIDByCommunicationID 根据通信系统 ID 获取内部 user_id
// 注意：此函数主要用于 websocket 通知等内部场景，业务逻辑应使用 communication_id
func GetUserIDByCommunicationID(userCommunicationID string) (string, error) {
	service, err := database.GetServiceForRelationship()
	if err != nil {
		return "", err
	}
	return database.GetUserIDByCommunicationID(service, userCommunicationID)
}