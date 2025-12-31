// Package configs 提供统一的路径配置管理服务
//
// 主要功能：
//   - 提供全局路径配置管理（单例模式）
//   - 支持多路径回退机制，自动查找有效路径
//   - 统一配置文件读取逻辑，避免重复代码
//
// 核心组件：
//   - PathConfig: 路径配置结构体，管理各种文件路径
//   - InitViperConfig: 统一的配置文件初始化函数
//
// 使用示例：
//   pc := configs.GetInstance()
//   configPath := pc.GetConfigPath("database.ini")
//   keyPath := pc.GetAsymmetricKeysPath()
package configs

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
)

// InitViperConfig 初始化Viper配置读取器
//
// 功能说明：
//   提供统一的配置文件读取逻辑，避免在多个地方重复编写相同的viper初始化代码
//
// 配置策略：
//   - 配置文件名：backendserviceconfig.ini
//   - 配置文件类型：INI格式
//   - 路径优先级：相对路径 -> Linux生产环境 -> Linux旧版路径 -> 简化路径 -> 备用相对路径
//
// 返回值：
//   error: 如果所有路径都无法读取配置文件，则返回错误
func InitViperConfig() error {
	// 配置Viper读取配置文件
	viper.SetConfigName("backendserviceconfig")
	viper.SetConfigType("ini")

	// 尝试不同的配置文件路径（按优先级排序）
	configPaths := []string{
		filepath.Join("backend", "common", "configs"),           // 相对路径（开发环境）
		"/program/digitalsingularity/backend/common/configs",    // Linux生产环境
		"/opt/digitalsingularity/backend/common/configs",        // Linux旧版路径
		"configs",                                                // 简化路径
		filepath.Join("backend", "configs"),                     // 备用相对路径
	}

	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	// 读取配置文件
	return viper.ReadInConfig()
}

var (
	instance *PathConfig
	once     sync.Once
	logger   = log.New(log.Writer(), "pathconfig: ", log.LstdFlags)
)

// PathConfig 路径配置结构体
//
// 功能说明：
//   管理应用程序中各种文件和目录的路径配置
//   支持主路径和备用路径的双重配置，确保系统兼容性
//
// 路径字段说明：
//   - BasePath: 应用程序基础路径（主路径）
//   - ConfigPath: 配置文件目录路径（主路径）
//   - AsymmetricKeysPath: 非对称密钥文件路径（主路径）
//   - BasePathLegacy: 备用基础路径（旧版本兼容）
//   - ConfigPathLegacy: 备用配置路径（旧版本兼容）
//   - AsymmetricKeysPathLegacy: 备用密钥路径（旧版本兼容）
type PathConfig struct {
	// 基础路径
	BasePath       string
	ConfigPath     string
	AsymmetricKeysPath string

	// 备用路径（旧版本兼容）
	BasePathLegacy       string
	ConfigPathLegacy     string
	AsymmetricKeysPathLegacy string
}

// GetInstance 获取PathConfig单例实例
//
// 功能说明：
//   返回全局唯一的PathConfig实例，使用sync.Once确保线程安全
//
// 返回值：
//   *PathConfig: 路径配置实例，首次调用时会自动初始化配置
//
// 使用示例：
//   pc := configs.GetInstance()
//   configPath := pc.GetConfigPath("app.ini")
func GetInstance() *PathConfig {
	once.Do(func() {
		instance = &PathConfig{}
		instance.loadConfig()
	})
	return instance
}

// loadConfig 从配置文件加载路径配置
//
// 功能说明：
//   从backendserviceconfig.ini配置文件中读取路径配置
//   如果配置文件读取失败，则使用默认路径设置
//
// 配置读取策略：
//   1. 调用InitViperConfig初始化viper配置
//   2. 从配置文件[Path]节读取路径配置
//   3. 如果主路径为空，则调用setDefaultPaths设置默认值
//
// 日志输出：
//   成功时输出路径配置信息
//   失败时输出警告信息并使用默认路径
func (p *PathConfig) loadConfig() {
	// 初始化Viper配置
	if err := InitViperConfig(); err != nil {
		logger.Printf("警告: 无法读取配置文件: %v, 使用默认路径", err)
		p.setDefaultPaths()
		return
	}

	// 从配置文件读取路径
	p.BasePath = viper.GetString("Path.base_path")
	p.ConfigPath = viper.GetString("Path.configs_path")
	p.AsymmetricKeysPath = viper.GetString("Path.asymmetric_keys_path")
	
	p.BasePathLegacy = viper.GetString("Path.base_path_legacy")
	p.ConfigPathLegacy = viper.GetString("Path.configs_path_legacy")
	p.AsymmetricKeysPathLegacy = viper.GetString("Path.asymmetric_keys_path_legacy")

	// 如果主路径为空，使用默认值
	if p.BasePath == "" {
		p.setDefaultPaths()
	}

	logger.Printf("路径配置加载完成: BasePath=%s, ConfigPath=%s", p.BasePath, p.ConfigPath)
}

// setDefaultPaths 设置默认路径配置
//
// 功能说明：
//   当配置文件无法读取或路径配置为空时，设置默认的路径值
//
// 默认路径说明：
//   - BasePath: /program/digitalsingularity （Linux生产环境路径）
//   - ConfigPath: /program/digitalsingularity/backend/common/configs
//   - AsymmetricKeysPath: 对应的密钥文件路径
//   - Legacy路径：对应的旧版本兼容路径（/opt/digitalsingularity）
func (p *PathConfig) setDefaultPaths() {
	p.BasePath = "/program/digitalsingularity"
	p.ConfigPath = "/program/digitalsingularity/backend/common/configs"
	p.AsymmetricKeysPath = "/program/digitalsingularity/backend/common/security/asymmetricencryption/keys/asymmetrickeys.json"
	
	p.BasePathLegacy = "/opt/digitalsingularity"
	p.ConfigPathLegacy = "/opt/digitalsingularity/backend/common/configs"
	p.AsymmetricKeysPathLegacy = "/opt/digitalsingularity/backend/common/security/asymmetricencryption/keys/asymmetrickeys.json"
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
// 日志输出：
//   当使用备用路径时会输出相应的日志信息
func (p *PathConfig) GetConfigPath(filename string) string {
	// 尝试主路径
	mainPath := filepath.Join(p.ConfigPath, filename)
	if _, err := os.Stat(mainPath); err == nil {
		return mainPath
	}

	// 尝试备用路径
	legacyPath := filepath.Join(p.ConfigPathLegacy, filename)
	if _, err := os.Stat(legacyPath); err == nil {
		logger.Printf("使用备用配置路径: %s", legacyPath)
		return legacyPath
	}

	// 尝试相对路径
	relativePath := filepath.Join("backend", "common", "configs", filename)
	if _, err := os.Stat(relativePath); err == nil {
		logger.Printf("使用相对配置路径: %s", relativePath)
		return relativePath
	}

	// 尝试当前工作目录
	if pwd := os.Getenv("PWD"); pwd != "" {
		pwdPath := filepath.Join(pwd, "backend", "common", "configs", filename)
		if _, err := os.Stat(pwdPath); err == nil {
			logger.Printf("使用工作目录配置路径: %s", pwdPath)
			return pwdPath
		}
	}

	// 如果都不存在，返回主路径（让调用者处理错误）
	logger.Printf("配置文件不存在，返回默认路径: %s", mainPath)
	return mainPath
}

// GetAsymmetricKeysPath 获取非对称密钥文件路径，自动检查文件是否存在
//
// 功能说明：
//   智能查找并返回有效的非对称密钥文件路径
//   采用多级回退策略，与GetConfigPath类似但专门针对密钥文件
//
// 路径查找优先级：
//   1. 主密钥路径 (AsymmetricKeysPath)
//   2. 备用密钥路径 (AsymmetricKeysPathLegacy)
//   3. 相对路径 (backend/common/security/asymmetricencryption/keyserializer/asymmetrickeys.json)
//   4. 当前工作目录路径 (PWD + 相对路径)
//
// 返回值：
//   string: 第一个找到的密钥文件完整路径，如果都找不到则返回主路径
//
// 日志输出：
//   当使用备用路径时会输出相应的日志信息
func (p *PathConfig) GetAsymmetricKeysPath() string {
	// 尝试主路径
	if _, err := os.Stat(p.AsymmetricKeysPath); err == nil {
		return p.AsymmetricKeysPath
	}

	// 尝试备用路径
	if _, err := os.Stat(p.AsymmetricKeysPathLegacy); err == nil {
		logger.Printf("使用备用密钥路径: %s", p.AsymmetricKeysPathLegacy)
		return p.AsymmetricKeysPathLegacy
	}

	// 尝试相对路径
	relativePath := filepath.Join("backend", "common", "security", "asymmetricencryption", "keyserializer", "asymmetrickeys.json")
	if _, err := os.Stat(relativePath); err == nil {
		logger.Printf("使用相对密钥路径: %s", relativePath)
		return relativePath
	}

	// 尝试当前工作目录
	if pwd := os.Getenv("PWD"); pwd != "" {
		pwdPath := filepath.Join(pwd, "backend", "common", "security", "asymmetricencryption", "keyserializer", "asymmetrickeys.json")
		if _, err := os.Stat(pwdPath); err == nil {
			logger.Printf("使用工作目录密钥路径: %s", pwdPath)
			return pwdPath
		}
	}

	// 如果都不存在，返回主路径
	logger.Printf("密钥文件不存在，返回默认路径: %s", p.AsymmetricKeysPath)
	return p.AsymmetricKeysPath
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
// 日志输出：
//   找到有效路径时输出日志
//   所有路径都不存在时输出警告日志
//
// 使用示例：
//   paths := []string{"/etc/myapp/config.ini", "./config.ini", "~/myapp/config.ini"}
//   validPath := pc.TryPaths(paths)
func (p *PathConfig) TryPaths(paths []string) string {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			logger.Printf("找到有效路径: %s", path)
			return path
		}
	}
	
	// 如果都不存在，返回第一个路径
	if len(paths) > 0 {
		logger.Printf("所有路径都不存在，返回第一个路径: %s", paths[0])
		return paths[0]
	}
	
	return ""
}

