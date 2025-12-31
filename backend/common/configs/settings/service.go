// Package settings 提供应用程序设置配置管理服务
//
// 主要功能：
//   - 管理应用程序的核心业务配置（数据库、Redis、JWT等）
//   - 支持配置文件和环境变量的双重配置来源
//   - 提供配置的默认值和回退机制
//
// 核心组件：
//   - CommonSettings: 应用程序设置结构体
//   - NewCommonSettings: 创建设置实例的工厂函数
//
// 配置来源优先级：
//   1. 环境变量（最高优先级，适用于容器化部署）
//   2. 配置文件 (backendserviceconfig.ini)
//   3. 默认值（最低优先级，保证程序能正常运行）
//
// 使用示例：
//   settings := settings.NewCommonSettings()
//   dbHost := settings.DbHost
//   redisConfig := fmt.Sprintf("%s:%d", settings.RedisHost, settings.RedisPort)
package settings

import (
	"fmt"
	"log"
	"os"

	pathconfig "digitalsingularity/backend/common/configs"

	"github.com/spf13/viper"
)

// CommonSettings 应用程序通用设置结构体
//
// 功能说明：
//   包含应用程序运行所需的所有核心配置信息
//   支持从配置文件和环境变量加载配置，提供默认值回退
//   同时提供路径查找功能，避免直接依赖路径配置服务
//
// 配置字段说明：
//   数据库配置：
//     - DbHost: 数据库主机地址
//     - DbPort: 数据库端口号
//     - DbUser: 数据库用户名
//     - DbPassword: 数据库密码
//     - DbName: 通用数据数据库名（默认为"common"）
//     - DbNameSilicoid: 硅基智能数据库名（默认为"silicoid"）
//
//   Redis配置：
//     - RedisHost: Redis服务器地址
//     - RedisPort: Redis端口号
//     - RedisPassword: Redis密码
//     - RedisDb: Redis数据库编号（默认0）
//
//   JWT配置：
//     - JwtSecret: JWT签名密钥
//     - JwtExpire: JWT过期时间（秒，默认3600秒）
//
//   其他配置：
//     - Debug: 是否启用调试模式
type CommonSettings struct {
	// 数据库配置
	DbHost               string
	DbPort               int
	DbUser               string
	DbPassword           string
	DbName               string
	DbNameSilicoid       string
	DbNameCommunication  string

	// Redis配置
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDb       int

	// JWT配置
	JwtSecret string
	JwtExpire int

	// 其他配置
	Debug bool

	// 内部使用的路径配置实例
	pathConfig *pathconfig.PathConfig
}

// NewCommonSettings 创建并返回一个新的CommonSettings实例
//
// 功能说明：
//   工厂函数，创建CommonSettings实例并自动加载配置
//
// 配置加载流程：
//   1. 创建CommonSettings实例
//   2. 初始化路径配置实例
//   3. 调用loadConfig()方法加载配置文件
//   4. 如果配置加载失败，使用默认值
//   5. 调用loadFromEnvironment()加载环境变量覆盖配置
//
// 返回值：
//   *CommonSettings: 配置完整的设置实例
//
// 使用示例：
//   settings := settings.NewCommonSettings()
//   fmt.Printf("Database: %s:%d\n", settings.DbHost, settings.DbPort)
func NewCommonSettings() *CommonSettings {
	settings := &CommonSettings{
		pathConfig: pathconfig.GetInstance(),
	}
	settings.loadConfig()
	return settings
}

// loadConfig 从配置文件加载配置
//
// 功能说明：
//   从backendserviceconfig.ini配置文件中读取应用程序设置
//   如果配置文件读取失败，则调用setDefaultValues设置默认值
//
// 配置读取说明：
//   - 数据库配置：从[database]节读取，使用配置文件中定义的数据库名称
//   - Redis配置：从[Redis]节读取
//   - JWT配置：从[TOKEN]节读取（secretKey字段）
//   - 调试配置：从[app]节读取（可选）
//
// 错误处理：
//   配置文件读取失败时记录警告日志并使用默认值
func (s *CommonSettings) loadConfig() {
	// 使用统一的配置初始化
	if err := pathconfig.InitViperConfig(); err != nil {
		log.Printf("警告: 无法读取配置文件: %v", err)
		s.setDefaultValues()
		return
	}

	// 设置数据库配置 - 使用配置文件中定义的数据库名称
	s.DbHost = viper.GetString("database.host")
	s.DbPort = viper.GetInt("database.port")
	s.DbUser = viper.GetString("database.user")
	s.DbPassword = viper.GetString("database.password")
	// 使用配置文件中的 data_common_database = common
	s.DbName = viper.GetString("database.data_common_database")
	// 使用配置文件中的 app_silicoid_database = silicoid
	s.DbNameSilicoid = viper.GetString("database.app_silicoid_database")
	// 使用配置文件中的 communication_system_database = communication_system
	s.DbNameCommunication = viper.GetString("database.communication_system_database")

	// 如果数据库名为空，使用默认值
	if s.DbName == "" {
		s.DbName = "common"
	}
	if s.DbNameSilicoid == "" {
		s.DbNameSilicoid = "silicoid"
	}

	// 设置Redis配置
	s.RedisHost = viper.GetString("Redis.host")
	s.RedisPort = viper.GetInt("Redis.port")
	s.RedisPassword = viper.GetString("Redis.pwd")
	s.RedisDb = viper.GetInt("Redis.db")

	// 设置JWT配置 - 使用配置文件中的TOKEN节
	s.JwtSecret = viper.GetString("TOKEN.secretKey")
	// 如果没有设置过期时间，使用默认值3600秒（1小时）
	if expire := viper.GetInt("TOKEN.expire"); expire > 0 {
		s.JwtExpire = expire
	} else {
		s.JwtExpire = 3600 // 默认1小时
	}

	// 设置调试配置
	s.Debug = viper.GetBool("app.debug")
}

// setDefaultValues 设置默认配置值
//
// 功能说明：
//   当配置文件无法读取时，设置合理的默认配置值
//   确保应用程序能够正常启动和运行
//
// 默认值说明：
//   - 数据库：localhost:3306, root/root, common/silicoid数据库
//   - Redis：localhost:6379, 无密码, db=0
//   - JWT：默认密钥，过期时间3600秒
//   - Debug：false（生产环境模式）
//
// 注意：
//   此函数还会调用loadFromEnvironment()，允许环境变量覆盖默认值
func (s *CommonSettings) setDefaultValues() {
	// 数据库默认值 - 使用配置文件中定义的数据库名称
	s.DbHost = "localhost"
	s.DbPort = 3306
	s.DbUser = "root"
	s.DbPassword = "root"
	s.DbName = "common"                    // data_common_database = common
	s.DbNameSilicoid = "silicoid"          // app_silicoid_database = silicoid
	s.DbNameCommunication = "communication_system"  // communication_system_database = communication_system

	// Redis默认值
	s.RedisHost = "localhost"
	s.RedisPort = 6379
	s.RedisPassword = ""
	s.RedisDb = 0

	// JWT默认值
	s.JwtSecret = "default_jwt_secret"
	s.JwtExpire = 3600

	// 其他默认值
	s.Debug = false

	// 尝试从环境变量获取值
	s.loadFromEnvironment()
}

// loadFromEnvironment 从环境变量加载配置
//
// 功能说明：
//   从系统环境变量中读取配置，支持容器化部署和动态配置
//   环境变量具有最高优先级，会覆盖配置文件和默认值
//
// 支持的环境变量：
//   数据库相关：
//     - DB_HOST: 数据库主机
//     - DB_PORT: 数据库端口
//     - DB_USER: 数据库用户
//     - DB_PASSWORD: 数据库密码
//     - DB_NAME: 通用数据库名
//     - DB_NAME_SILICOID: 硅基智能数据库名
//
//   Redis相关：
//     - REDIS_HOST: Redis主机
//     - REDIS_PORT: Redis端口
//     - REDIS_PASSWORD: Redis密码
//     - REDIS_DB: Redis数据库编号
//
//   JWT相关：
//     - JWT_SECRET: JWT密钥
//     - JWT_EXPIRE: JWT过期时间（秒）
//
//   其他：
//     - DEBUG: 调试模式（true/1/yes启用，其他值为false）
//
// 注意：
//   数值类型的环境变量会自动解析，解析失败则忽略
func (s *CommonSettings) loadFromEnvironment() {
	// 数据库环境变量
	if val, exists := os.LookupEnv("DB_HOST"); exists {
		s.DbHost = val
	}
	if val, exists := os.LookupEnv("DB_PORT"); exists {
		if port, err := parseInt(val); err == nil {
			s.DbPort = port
		}
	}
	if val, exists := os.LookupEnv("DB_USER"); exists {
		s.DbUser = val
	}
	if val, exists := os.LookupEnv("DB_PASSWORD"); exists {
		s.DbPassword = val
	}
	if val, exists := os.LookupEnv("DB_NAME"); exists {
		s.DbName = val
	}
	if val, exists := os.LookupEnv("DB_NAME_SILICOID"); exists {
		s.DbNameSilicoid = val
	}
	if val, exists := os.LookupEnv("DB_NAME_COMMUNICATION"); exists {
		s.DbNameCommunication = val
	}

	// Redis环境变量
	if val, exists := os.LookupEnv("REDIS_HOST"); exists {
		s.RedisHost = val
	}
	if val, exists := os.LookupEnv("REDIS_PORT"); exists {
		if port, err := parseInt(val); err == nil {
			s.RedisPort = port
		}
	}
	if val, exists := os.LookupEnv("REDIS_PASSWORD"); exists {
		s.RedisPassword = val
	}
	if val, exists := os.LookupEnv("REDIS_DB"); exists {
		if db, err := parseInt(val); err == nil {
			s.RedisDb = db
		}
	}

	// JWT环境变量
	if val, exists := os.LookupEnv("JWT_SECRET"); exists {
		s.JwtSecret = val
	}
	if val, exists := os.LookupEnv("JWT_EXPIRE"); exists {
		if expire, err := parseInt(val); err == nil {
			s.JwtExpire = expire
		}
	}

	// 调试环境变量
	if val, exists := os.LookupEnv("DEBUG"); exists {
		s.Debug = val == "true" || val == "1" || val == "yes"
	}
}

// GetConfigPath 获取配置文件路径，自动检查文件是否存在
//
// 功能说明：
//   根据指定的文件名，智能查找并返回有效的配置文件路径
//   采用多级回退策略，确保在各种环境下都能找到正确的配置文件
//
// 路径查找优先级：
//   1. 主配置路径 (ConfigPath + filename)
//   2. 备用配置路径 (ConfigPathLegacy + filename)
//   3. 相对路径 (backend/common/configs + filename)
//   4. 当前工作目录路径 (PWD + backend/common/configs + filename)
//
// 参数：
//   filename: 配置文件名（如"database.ini"）
//
// 返回值：
//   string: 第一个找到的配置文件完整路径，如果都找不到则返回主路径
//
// 使用示例：
//   settings := NewCommonSettings()
//   configPath := settings.GetConfigPath("server_public_key.pem")
func (s *CommonSettings) GetConfigPath(filename string) string {
	return s.pathConfig.GetConfigPath(filename)
}

// GetAsymmetricKeysPath 获取非对称密钥文件路径，自动检查文件是否存在
//
// 功能说明：
//   智能查找并返回有效的非对称密钥文件路径
//   采用多级回退策略，与GetConfigPath类似但专门针对密钥文件
//
// 返回值：
//   string: 第一个找到的密钥文件完整路径，如果都找不到则返回主路径
//
// 使用示例：
//   settings := NewCommonSettings()
//   keyPath := settings.GetAsymmetricKeysPath()
func (s *CommonSettings) GetAsymmetricKeysPath() string {
	return s.pathConfig.GetAsymmetricKeysPath()
}

// TryPaths 尝试多个路径，返回第一个存在的路径
//
// 功能说明：
//   遍历给定的路径列表，返回第一个存在的有效路径
//   通用路径查找工具函数，可用于各种文件类型的路径查找
//
// 参数：
//   paths: 待检查的路径字符串数组，按优先级排序
//
// 返回值：
//   string: 第一个存在的路径，如果都不存在则返回第一个路径（让调用者处理）
//
// 使用示例：
//   settings := NewCommonSettings()
//   paths := []string{"/etc/myapp/config.ini", "./config.ini"}
//   validPath := settings.TryPaths(paths)
func (s *CommonSettings) TryPaths(paths []string) string {
	return s.pathConfig.TryPaths(paths)
}

// parseInt 辅助函数：将字符串解析为整数
//
// 功能说明：
//   将字符串转换为整数，用于环境变量的数值解析
//
// 参数：
//   s: 待解析的字符串（如环境变量值）
//
// 返回值：
//   int: 解析后的整数值
//   error: 解析错误，如果字符串不是有效的整数格式
//
// 使用场景：
//   主要用于解析环境变量中的端口号、数据库编号等数值配置
//
// 示例：
//   port, err := parseInt("3306")  // 返回 3306, nil
//   port, err := parseInt("abc")   // 返回 0, error
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
} 