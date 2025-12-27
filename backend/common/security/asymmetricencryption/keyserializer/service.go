package keyserializer

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"digitalsingularity/backend/common/configs"
)

// LoadPrivateKeyFromFile 从文件中加载私钥
func LoadPrivateKeyFromFile(filePath, userName, passWord string) (*rsa.PrivateKey, error) {
	logger := log.New(os.Stdout, "security.keyserializer: ", log.LstdFlags)
	logger.Printf("开始加载私钥，文件路径参数: %s, 用户名: %s", filePath, userName)

	// 使用统一的路径配置获取配置文件路径
	pathCfg := configs.GetInstance()
	privateKeyFilename := fmt.Sprintf("%s_private_key.pem", userName)
	
	// 定义可能的私钥文件位置列表
	possibleLocations := []string{
		// 相对路径
		filepath.Join(filePath, privateKeyFilename),
		// backend/common/configs/路径
		filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filePath))), 
			"configs", privateKeyFilename),
		// 从配置获取的主路径
		pathCfg.GetConfigPath(privateKeyFilename),
		// 配置中的备用路径
		filepath.Join(pathCfg.ConfigPathLegacy, privateKeyFilename),
	}

	// 逐个尝试可能的位置
	for _, privateKeyPath := range possibleLocations {
		logger.Printf("尝试从路径加载私钥: %s", privateKeyPath)

		if _, err := os.Stat(privateKeyPath); err == nil {
			privateKeyPem, err := ioutil.ReadFile(privateKeyPath)
			if err != nil {
				logger.Printf("从 %s 读取私钥文件失败: %s", privateKeyPath, err)
				continue
			}

			block, _ := pem.Decode(privateKeyPem)
			if block == nil {
				logger.Printf("私钥PEM解码失败")
				continue
			}

			var privateKeyBytes []byte

			if passWord != "" {
				// 如果提供了密码，使用密码解密私钥
				decrypted, decErr := x509.DecryptPEMBlock(block, []byte(passWord))
				if decErr != nil {
					logger.Printf("私钥解密失败: %s", decErr)
					continue
				}
				privateKeyBytes = decrypted
			} else {
				// 否则不使用密码
				privateKeyBytes = block.Bytes
			}

			// 使用PKCS8格式解析私钥
			parsedKey, err := x509.ParsePKCS8PrivateKey(privateKeyBytes)
			if err != nil {
				logger.Printf("私钥数据解析失败: %s", err)
				continue
			}

			// 将解析后的密钥转换为RSA私钥
			privateKey, ok := parsedKey.(*rsa.PrivateKey)
			if !ok {
				logger.Printf("非RSA私钥类型，无法使用")
				continue
			}

			logger.Printf("私钥加载成功，文件路径: %s", privateKeyPath)
			return privateKey, nil
		}
	}

	// 所有可能位置都尝试失败
	errorMsg := fmt.Sprintf("无法找到私钥文件，尝试了以下路径: %v", possibleLocations)
	logger.Printf(errorMsg)
	return nil, fmt.Errorf(errorMsg)
} 