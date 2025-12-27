// Package pathconfig 提供统一的路径配置管理
package configs

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
)

var (
	instance *PathConfig
	once     sync.Once
	logger   = log.New(log.Writer(), "pathconfig: ", log.LstdFlags)
)

// PathConfig 路径配置结构
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
func GetInstance() *PathConfig {
	once.Do(func() {
		instance = &PathConfig{}
		instance.loadConfig()
	})
	return instance
}

// loadConfig 从配置文件加载路径配置
func (p *PathConfig) loadConfig() {
	// 配置Viper读取配置文件
	viper.SetConfigName("backendserviceconfig")
	viper.SetConfigType("ini")

	// 尝试不同的配置文件路径
	configPaths := []string{
		filepath.Join("backend", "common", "configs"),
		"/program/digitalsingularity/backend/common/configs",
		"configs",
	}

	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
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

// setDefaultPaths 设置默认路径
func (p *PathConfig) setDefaultPaths() {
	p.BasePath = "/program/digitalsingularity"
	p.ConfigPath = "/program/digitalsingularity/backend/common/configs"
	p.AsymmetricKeysPath = "/program/digitalsingularity/backend/common/security/asymmetricencryption/keys/asymmetrickeys.json"
}

// GetConfigPath 获取配置文件路径，自动检查文件是否存在
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

