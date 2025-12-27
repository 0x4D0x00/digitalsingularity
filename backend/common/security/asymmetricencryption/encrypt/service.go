package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

// AsymmetricEncryptService 非对称加密服务
func AsymmetricEncryptService(inputString string, publicKey *rsa.PublicKey) (string, error) {
	// 生成加密操作的唯一ID
	encryptId := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	
	// 获取logger
	logger := log.New(os.Stdout, "security.encrypt: ", log.LstdFlags)
	
	// 记录开始加密
	inputPreview := inputString
	if len(inputString) > 100 {
		inputPreview = inputString[:100] + "..."
	}
	logger.Printf("[ENCRYPT:%s] 开始加密数据，原始数据长度: %d，内容预览: %s", encryptId, len(inputString), inputPreview)
	
	// 将输入字符串编码为字节串
	logger.Printf("[ENCRYPT:%s] 步骤1: 将输入字符串编码为字节串", encryptId)
	data := []byte(inputString)
	logger.Printf("[ENCRYPT:%s] 编码后数据长度: %d 字节", encryptId, len(data))

	// 生成随机AES密钥
	logger.Printf("[ENCRYPT:%s] 步骤4: 生成随机AES密钥", encryptId)
	aesKey := make([]byte, 16) // 使用AES-128
	_, err := rand.Read(aesKey)
	if err != nil {
		logger.Printf("[ENCRYPT:%s] AES密钥生成失败: %s", encryptId, err)
		return "", fmt.Errorf("加密失败: %v", err)
	}
	logger.Printf("[ENCRYPT:%s] AES密钥生成成功，长度: %d 字节", encryptId, len(aesKey))

	// 使用AES-CBC模式加密数据
	logger.Printf("[ENCRYPT:%s] 步骤5: 使用AES-CBC模式加密数据", encryptId)
	iv := make([]byte, aes.BlockSize) // CBC模式需要16字节的IV（完整的AES块）
	_, err = rand.Read(iv)
	if err != nil {
		logger.Printf("[ENCRYPT:%s] IV生成失败: %s", encryptId, err)
		return "", fmt.Errorf("加密失败: %v", err)
	}
	logger.Printf("[ENCRYPT:%s] 生成IV，长度: %d 字节", encryptId, len(iv))
	
	// 添加PKCS#7填充
	logger.Printf("[ENCRYPT:%s] 添加PKCS#7填充", encryptId)
	paddedData := pkcs7Pad(data, aes.BlockSize)
	logger.Printf("[ENCRYPT:%s] 填充后数据长度: %d 字节", encryptId, len(paddedData))
	
	// 使用CBC模式加密
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		logger.Printf("[ENCRYPT:%s] 创建AES加密器失败: %s", encryptId, err)
		return "", fmt.Errorf("加密失败: %v", err)
	}
	
	ciphertext := make([]byte, len(paddedData))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, paddedData)
	
	logger.Printf("[ENCRYPT:%s] 数据加密成功，密文长度: %d 字节", encryptId, len(ciphertext))

	// 使用RSA公钥加密AES密钥
	logger.Printf("[ENCRYPT:%s] 步骤6: 使用RSA公钥加密AES密钥", encryptId)
	encryptedKey, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		publicKey,
		aesKey,
		nil,
	)
	if err != nil {
		logger.Printf("[ENCRYPT:%s] AES密钥加密失败: %s", encryptId, err)
		return "", fmt.Errorf("加密失败: %v", err)
	}
	logger.Printf("[ENCRYPT:%s] AES密钥加密成功，加密后长度: %d 字节", encryptId, len(encryptedKey))

	// 组合加密的AES密钥、IV和密文
	logger.Printf("[ENCRYPT:%s] 步骤7: 组合加密的AES密钥、IV和密文", encryptId)
	encryptedData := append(encryptedKey, iv...)
	encryptedData = append(encryptedData, ciphertext...)
	logger.Printf("[ENCRYPT:%s] 数据组合成功，总长度: %d 字节", encryptId, len(encryptedData))

	// 将组合后的数据进行16进制编码
	encryptedDataHex := hex.EncodeToString(encryptedData)
	logger.Printf("[ENCRYPT:%s] 16进制编码成功，编码后长度: %d", encryptId, len(encryptedDataHex))

	// 返回编码后的字符串
	logger.Printf("[ENCRYPT:%s] 加密完成，返回结果长度: %d", encryptId, len(encryptedDataHex))
	return encryptedDataHex, nil
}

// pkcs7Pad 添加PKCS#7填充
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := make([]byte, padding)
	for i := 0; i < padding; i++ {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
} 