package settings

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// CommonSettings 包含应用程序的配置设置
type CommonSettings struct {
	// 数据库配置
	DbHost         string
	DbPort         int
	DbUser         string
	DbPassword     string
	DbName         string
	DbNameSilicoid string

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
}

// NewCommonSettings 创建并返回一个新的CommonSettings实例
func NewCommonSettings() *CommonSettings {
	settings := &CommonSettings{}
	settings.loadConfig()
	return settings
}

// 从配置文件加载配置
func (s *CommonSettings) loadConfig() {
	// 配置Viper读取配置文件
	viper.SetConfigName("backendserviceconfig")
	viper.SetConfigType("ini")

	// 尝试不同的配置文件路径
	configPaths := []string{
		filepath.Join("backend", "common", "configs"),
		filepath.Join("backend", "configs"),
		"configs",
	}

	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
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

	// 设置JWT配置
	s.JwtSecret = viper.GetString("JWT.secret")
	s.JwtExpire = viper.GetInt("JWT.expire")

	// 设置调试配置
	s.Debug = viper.GetBool("app.debug")
}

// 设置默认值（当配置文件无法读取时使用）
func (s *CommonSettings) setDefaultValues() {
	// 数据库默认值 - 使用配置文件中定义的数据库名称
	s.DbHost = "localhost"
	s.DbPort = 3306
	s.DbUser = "root"
	s.DbPassword = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	s.DbName = "common"              // data_common_database = common
	s.DbNameSilicoid = "silicoid"    // app_silicoid_database = silicoid

	// Redis默认值
	s.RedisHost = "localhost"
	s.RedisPort = 6379
	s.RedisPassword = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	s.RedisDb = 0

	// JWT默认值
	s.JwtSecret = "default_jwt_secret"
	s.JwtExpire = 3600

	// 其他默认值
	s.Debug = false

	// 尝试从环境变量获取值
	s.loadFromEnvironment()
}

// 从环境变量加载配置
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

// 辅助函数:将字符串解析为整数
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
} 