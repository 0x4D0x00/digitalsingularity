package decrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"digitalsingularity/backend/common/security/asymmetricencryption/keyserializer"
)

// AsymmetricDecryptService 非对称解密服务
func AsymmetricDecryptService(encryptedData string, info map[string]string) (string, error) {
	// 生成解密操作的唯一ID
	decryptId := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	
	// 获取logger
	logger := log.New(os.Stdout, "security.decrypt: ", log.LstdFlags)
	
	// 记录开始解密
	logger.Printf("[DECRYPT:%s] 开始解密数据，长度: %d", decryptId, len(encryptedData))
	
	// 步骤1: 解码16进制加密数据
	logger.Printf("[DECRYPT:%s] 步骤1: 解码16进制加密数据", decryptId)
	
	// 16进制解码
	logger.Printf("[DECRYPT:%s] 使用16进制解码", decryptId)
	decodedData, err := hex.DecodeString(encryptedData)
	if err != nil {
		logger.Printf("[DECRYPT:%s] 解码失败: %s", decryptId, err)
		return "", fmt.Errorf("16进制解码失败: %v", err)
	}
	
	logger.Printf("[DECRYPT:%s] 解码后数据长度: %d 字节", decryptId, len(decodedData))
	
	// 步骤2: 从配置加载私钥
	if info == nil {
		logger.Printf("[DECRYPT:%s] 参数错误: info 为空", decryptId)
		return "", fmt.Errorf("解密参数错误: info 必须是包含文件路径和用户名的字典")
	}
	
	filePath := info["filePath"]
	userName := info["userName"]
	passWord := info["passWord"]
	
	logger.Printf("[DECRYPT:%s] 步骤2: 从文件加载私钥，文件路径: %s, 用户名: %s", decryptId, filePath, userName)
	// 使用 LoadPrivateKeyFromFile 函数
	privateKey, err := keyserializer.LoadPrivateKeyFromFile(filePath, userName, passWord)
	if err != nil {
		logger.Printf("[DECRYPT:%s] 私钥加载失败: %s", decryptId, err)
		return "", fmt.Errorf("私钥加载失败: %v", err)
	}
	
	logger.Printf("[DECRYPT:%s] 私钥加载成功", decryptId)

	// 步骤3: 提取加密的 AES 密钥、IV 和密文
	keySizeBytes := privateKey.Size() // 私钥长度（字节），通常为 256 字节（2048 位）
	ivLength := 16                    // CBC 模式 IV 长度为 16 字节

	logger.Printf("[DECRYPT:%s] 步骤3: 提取数据组件，密钥大小: %d 字节, IV长度: %d 字节", decryptId, keySizeBytes, ivLength)
	
	// 确保解码的数据长度足够
	if len(decodedData) < (keySizeBytes + ivLength) {
		errorMsg := fmt.Sprintf("解码后数据长度不足: %d 字节，预期至少 %d 字节", len(decodedData), keySizeBytes+ivLength)
		logger.Printf("[DECRYPT:%s] %s", decryptId, errorMsg)
		return "", fmt.Errorf(errorMsg)
	}
	
	encryptedKey := decodedData[:keySizeBytes]
	iv := decodedData[keySizeBytes:keySizeBytes+ivLength]
	ciphertext := decodedData[keySizeBytes+ivLength:]
	
	logger.Printf("[DECRYPT:%s] 提取加密密钥长度: %d 字节", decryptId, len(encryptedKey))
	logger.Printf("[DECRYPT:%s] 提取IV长度: %d 字节", decryptId, len(iv))
	logger.Printf("[DECRYPT:%s] 提取密文长度: %d 字节", decryptId, len(ciphertext))

	// 步骤4: 使用 RSA 私钥解密 AES 密钥
	logger.Printf("[DECRYPT:%s] 步骤4: 使用RSA私钥解密AES密钥", decryptId)
	aesKey, err := rsa.DecryptOAEP(
		sha256.New(),
		nil,
		privateKey,
		encryptedKey,
		nil,
	)
	if err != nil {
		logger.Printf("[DECRYPT:%s] AES密钥解密失败: %s", decryptId, err)
		return "", fmt.Errorf("AES密钥解密失败: %v", err)
	}
	logger.Printf("[DECRYPT:%s] AES密钥解密成功，长度: %d 字节", decryptId, len(aesKey))

	// 步骤5: 使用 AES-CBC 解密数据
	logger.Printf("[DECRYPT:%s] 步骤5: 使用AES-CBC解密数据", decryptId)
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		logger.Printf("[DECRYPT:%s] 创建AES解密器失败: %s", decryptId, err)
		return "", fmt.Errorf("数据解密失败: %v", err)
	}
	
	mode := cipher.NewCBCDecrypter(block, iv)
	paddedData := make([]byte, len(ciphertext))
	mode.CryptBlocks(paddedData, ciphertext)
	
	// 使用PKCS7解除填充
	decryptedData, err := pkcs7Unpad(paddedData, aes.BlockSize)
	if err != nil {
		logger.Printf("[DECRYPT:%s] PKCS7解除填充失败: %s", decryptId, err)
		return "", fmt.Errorf("数据解密失败: %v", err)
	}
	
	logger.Printf("[DECRYPT:%s] 数据解密成功，解密后长度: %d 字节", decryptId, len(decryptedData))
	
	// 步骤6: 返回解密后的字符串
	result := string(decryptedData)
	// 只记录前100个字符，避免日志过大
	preview := result
	if len(result) > 100 {
		preview = result[:100] + "..."
	}
	logger.Printf("[DECRYPT:%s] 步骤6: 解密完成，返回解密后字符串，前100个字符: %s", decryptId, preview)
	return result, nil
}

// pkcs7Unpad 解除PKCS#7填充
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("无效的填充数据，长度为0")
	}
	
	padding := int(data[len(data)-1])
	
	if padding > blockSize || padding == 0 {
		return nil, fmt.Errorf("无效的填充值")
	}
	
	// 检查所有填充字节
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("填充格式不正确")
		}
	}
	
	return data[:len(data)-padding], nil
} 