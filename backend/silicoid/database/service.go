package database

import (
	"log"

	"digitalsingularity/backend/common/utils/datahandle"
)

// SilicoidDataService 处理Silicoid应用相关的数据服务
type SilicoidDataService struct {
	readWrite *datahandle.CommonReadWriteService
	dbName    string
}

// NewSilicoidDataService 创建一个新的SilicoidDataService实例
func NewSilicoidDataService(readWrite *datahandle.CommonReadWriteService) *SilicoidDataService {
	service := &SilicoidDataService{
		readWrite: readWrite,
		dbName:    "silicoid", // 默认值
	}

	// 配置从配置文件中读取数据库名的逻辑已移除
	// 使用默认值 "silicoid"
	log.Printf("[Silicoid] 使用默认数据库名: %s", service.dbName)

	return service
}

// GetDatabaseName 返回当前使用的数据库名
func (s *SilicoidDataService) GetDatabaseName() string {
	return s.dbName
}