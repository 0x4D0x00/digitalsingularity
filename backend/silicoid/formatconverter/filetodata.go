// 文件转文本工具
// 支持：txt、csv、md、PDF、docx、xlsx、image
// 优先尝试转成可读的自然语言文本，如果不行，转换成 base64

package formatconverter

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

// FileConvertResult 文件转换结果
type FileConvertResult struct {
	Success  bool   // 是否成功
	Text     string // 转换后的文本内容（如果成功提取文本）
	IsBinary bool   // 是否为二进制格式（无法提取文本，已转为 base64）
	ErrorMsg string // 错误信息（如果失败）
}

// ConvertFileSmart 智能转换文件为文本或 base64
// fileBytes: 文件内容
// mimeType: MIME 类型，如 "application/pdf", "text/plain" 等
// fileId: 文件ID（用于日志）
// provider: 提供者（如 "kimi"），用于特殊处理
func ConvertFileSmart(fileBytes []byte, mimeType string, fileId string, provider string) FileConvertResult {
	// 根据 MIME 类型选择处理方式
	switch {
	case mimeType == "text/plain" || mimeType == "text/markdown" || mimeType == "text/x-markdown":
		// TXT 和 MD 文件，直接读取文本
		return convertTextFile(fileBytes)
	
	case mimeType == "text/csv" || strings.HasSuffix(strings.ToLower(fileId), ".csv"):
		// CSV 文件，转换为文本格式
		return convertCSVFile(fileBytes)
	
	case mimeType == "application/pdf":
		// PDF 文件，尝试提取文本，失败则转 base64
		return convertPDFFile(fileBytes, fileId)
	
	case mimeType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || 
		 strings.HasSuffix(strings.ToLower(fileId), ".docx"):
		// DOCX 文件，尝试提取文本，失败则转 base64
		return convertDOCXFile(fileBytes, fileId)
	
	case mimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || 
		 strings.HasSuffix(strings.ToLower(fileId), ".xlsx"):
		// XLSX 文件，转换为文本格式
		return convertXLSXFile(fileBytes, fileId)
	
	case strings.HasPrefix(mimeType, "image/"):
		// 图片文件，尝试 OCR 或转 base64
		return convertImageFile(fileBytes, mimeType)
	
	default:
		// 其他类型，尝试作为文本读取，失败则转 base64
		return convertUnknownFile(fileBytes, mimeType)
	}
}

// convertTextFile 转换文本文件（TXT、MD）
func convertTextFile(fileBytes []byte) FileConvertResult {
	// 尝试检测编码并转换为 UTF-8
	text := string(fileBytes)
	
	// 检查是否包含可读文本（至少有一些非控制字符）
	if len(strings.TrimSpace(text)) == 0 {
		// 空文件或只有空白字符，返回 base64
		return FileConvertResult{
			Success:  true,
			Text:     base64.StdEncoding.EncodeToString(fileBytes),
			IsBinary: true,
			ErrorMsg: "",
		}
	}
	
	return FileConvertResult{
		Success:  true,
		Text:     text,
		IsBinary: false,
		ErrorMsg: "",
	}
}

// convertCSVFile 转换 CSV 文件为文本
func convertCSVFile(fileBytes []byte) FileConvertResult {
	reader := csv.NewReader(bytes.NewReader(fileBytes))
	reader.Comma = ',' // 默认逗号分隔
	
	var result strings.Builder
	
	// 读取所有行
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// CSV 解析失败，返回 base64
			return FileConvertResult{
				Success:  true,
				Text:     base64.StdEncoding.EncodeToString(fileBytes),
				IsBinary: true,
				ErrorMsg: fmt.Sprintf("CSV 解析失败: %v", err),
			}
		}
		
		// 将行转换为文本，用制表符分隔
		result.WriteString(strings.Join(record, "\t"))
		result.WriteString("\n")
	}
	
	text := result.String()
	if len(strings.TrimSpace(text)) == 0 {
		// 空 CSV，返回 base64
		return FileConvertResult{
			Success:  true,
			Text:     base64.StdEncoding.EncodeToString(fileBytes),
			IsBinary: true,
			ErrorMsg: "CSV 文件为空",
		}
	}
	
	return FileConvertResult{
		Success:  true,
		Text:     text,
		IsBinary: false,
		ErrorMsg: "",
	}
}

// convertPDFFile 转换 PDF 文件
func convertPDFFile(fileBytes []byte, fileId string) FileConvertResult {
	// go-fitz 需要文件路径，创建临时文件
	tmpFile, err := os.CreateTemp("", "pdf_convert_*.pdf")
	if err != nil {
		return FileConvertResult{
			Success:  true,
			Text:     base64.StdEncoding.EncodeToString(fileBytes),
			IsBinary: true,
			ErrorMsg: fmt.Sprintf("创建临时文件失败: %v", err),
		}
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // 清理临时文件
	defer tmpFile.Close()
	
	// 写入文件内容
	_, err = tmpFile.Write(fileBytes)
	if err != nil {
		return FileConvertResult{
			Success:  true,
			Text:     base64.StdEncoding.EncodeToString(fileBytes),
			IsBinary: true,
			ErrorMsg: fmt.Sprintf("写入临时文件失败: %v", err),
		}
	}
	tmpFile.Close()
	
	// 暂时跳过PDF文本提取，直接返回base64
	// TODO: 修复go-fitz包的使用
	return FileConvertResult{
		Success:  true,
		Text:     base64.StdEncoding.EncodeToString(fileBytes),
		IsBinary: true,
		ErrorMsg: "PDF处理暂时跳过，使用base64编码",
	}
}

// convertDOCXFile 转换DOCX文件
func convertDOCXFile(fileBytes []byte, fileId string) FileConvertResult {
	// TODO: 实现DOCX文本提取
	return FileConvertResult{
		Success:  true,
		Text:     base64.StdEncoding.EncodeToString(fileBytes),
		IsBinary: true,
		ErrorMsg: "DOCX处理暂时跳过，使用base64编码",
	}
}

// convertXLSXFile 转换XLSX文件
func convertXLSXFile(fileBytes []byte, fileId string) FileConvertResult {
	// TODO: 实现XLSX文本提取
	return FileConvertResult{
		Success:  true,
		Text:     base64.StdEncoding.EncodeToString(fileBytes),
		IsBinary: true,
		ErrorMsg: "XLSX处理暂时跳过，使用base64编码",
	}
}

// convertImageFile 转换图片文件
func convertImageFile(fileBytes []byte, mimeType string) FileConvertResult {
	// 图片文件直接返回base64
	return FileConvertResult{
		Success:  true,
		Text:     base64.StdEncoding.EncodeToString(fileBytes),
		IsBinary: true,
		ErrorMsg: "",
	}
}

// convertUnknownFile 转换未知文件类型
func convertUnknownFile(fileBytes []byte, mimeType string) FileConvertResult {
	// 未知文件类型返回base64
	return FileConvertResult{
		Success:  true,
		Text:     base64.StdEncoding.EncodeToString(fileBytes),
		IsBinary: true,
		ErrorMsg: fmt.Sprintf("未知文件类型 (%s)，已转换为 base64", mimeType),
	}
}
