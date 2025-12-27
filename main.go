package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	mainhttp "digitalsingularity/backend/main/http"
	mainwebsocket "digitalsingularity/backend/main/websocket"
	silicoidhttp "digitalsingularity/backend/silicoid/http"
	//silicoidwebsocket "digitalsingularity/backend/silicoid/websocket"
	mcphttp "digitalsingularity/backend/modelcontextprotocol/http"
)

// 日志对象
var (
	logger      *log.Logger
	httpLogger  *log.Logger
	wsLogger    *log.Logger
	secLogger   *log.Logger
	loginLogger *log.Logger
)

// 设置日志
func setupLogging(logLevel string) {
	// 优先使用环境变量指定的日志目录
	// 如果没有设置，则使用用户主目录
	// 如果systemd以root运行，则使用/var/log目录
	var logDir string
	if envLogDir := os.Getenv("DIGITALSINGULARITY_LOG_DIR"); envLogDir != "" {
		logDir = envLogDir
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// 如果获取用户主目录失败，尝试使用/var/log
			if _, err := os.Stat("/var/log"); err == nil {
				logDir = "/var/log/digitalsingularity_logs"
			} else {
				logDir = "."
			}
		} else {
			// 如果以root用户运行（homeDir为/root），使用/var/log
			if homeDir == "/root" {
				logDir = "/var/log/digitalsingularity_logs"
			} else {
				logDir = filepath.Join(homeDir, "digitalsingularity_logs")
			}
		}
	}

	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("无法创建日志目录: %v\n", err)
		os.Exit(1)
	}

	// 主应用日志
	appLogFile, err := os.OpenFile(
		filepath.Join(logDir, "app.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err != nil {
		fmt.Printf("无法创建应用日志文件: %v\n", err)
		os.Exit(1)
	}

	// HTTP请求日志
	httpLogFile, err := os.OpenFile(
		filepath.Join(logDir, "http_requests.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err != nil {
		fmt.Printf("无法创建HTTP日志文件: %v\n", err)
		os.Exit(1)
	}

	// WebSocket日志
	wsLogFile, err := os.OpenFile(
		filepath.Join(logDir, "websocket.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err != nil {
		fmt.Printf("无法创建WebSocket日志文件: %v\n", err)
		os.Exit(1)
	}

	// 安全操作日志
	secLogFile, err := os.OpenFile(
		filepath.Join(logDir, "security.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err != nil {
		fmt.Printf("无法创建安全日志文件: %v\n", err)
		os.Exit(1)
	}

	// 设置日志格式
	logFlags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile

	// 主日志器 - 输出到控制台和文件
	multiWriter := io.MultiWriter(os.Stdout, appLogFile)
	logger = log.New(multiWriter, "[INFO] ", logFlags)

	// HTTP服务日志器
	httpLogger = log.New(httpLogFile, "[HTTP] ", logFlags)

	// WebSocket服务日志器
	wsLogger = log.New(wsLogFile, "[WS] ", logFlags)

	// 安全服务日志器
	secLogger = log.New(secLogFile, "[SEC] ", logFlags)

	// 登录尝试日志
	loginLogger = log.New(multiWriter, "[LOGIN] ", logFlags)

	// 打印日志配置信息
	logger.Printf("日志配置完成。日志文件保存在: %s", logDir)
	logger.Printf("主日志: %s", filepath.Join(logDir, "app.log"))
	logger.Printf("HTTP请求日志: %s", filepath.Join(logDir, "http_requests.log"))
	logger.Printf("WebSocket日志: %s", filepath.Join(logDir, "websocket.log"))
	logger.Printf("安全操作日志: %s", filepath.Join(logDir, "security.log"))
}

// ensureWorkingDirectory 确保工作目录正确
// 尝试切换到程序所在目录或项目根目录（/program/digitalsingularity 或 /opt/digitalsingularity）
func ensureWorkingDirectory() error {
	// 获取可执行文件的路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法获取可执行文件路径: %v", err)
	}

	// 获取可执行文件的绝对路径
	execAbsPath, err := filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("无法获取可执行文件绝对路径: %v", err)
	}

	// 获取可执行文件所在目录
	execDir := filepath.Dir(execAbsPath)

	// 尝试找到项目根目录（包含 main.go 或 go.mod 的目录）
	possibleDirs := []string{
		execDir,                                              // 可执行文件所在目录
		"/program/digitalsingularity",                       // 默认项目路径
		"/opt/digitalsingularity",                           // 备用项目路径
		filepath.Join(execDir, ".."),                        // 可执行文件上级目录
		filepath.Join(execDir, "../.."),                     // 可执行文件上两级目录
	}

	for _, dir := range possibleDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}

		// 检查是否是项目根目录（包含 main.go 或 go.mod）
		mainGoPath := filepath.Join(absDir, "main.go")
		goModPath := filepath.Join(absDir, "go.mod")

		if _, err := os.Stat(mainGoPath); err == nil {
			// 找到 main.go，切换到该目录
			if err := os.Chdir(absDir); err != nil {
				continue
			}
			fmt.Printf("工作目录已设置为: %s\n", absDir)
			return nil
		}

		if _, err := os.Stat(goModPath); err == nil {
			// 找到 go.mod，切换到该目录
			if err := os.Chdir(absDir); err != nil {
				continue
			}
			fmt.Printf("工作目录已设置为: %s\n", absDir)
			return nil
		}
	}

	// 如果都找不到，尝试切换到可执行文件所在目录
	if err := os.Chdir(execDir); err != nil {
		return fmt.Errorf("无法切换到可执行文件目录: %v", err)
	}

	fmt.Printf("工作目录已设置为可执行文件目录: %s\n", execDir)
	return nil
}

// HTTP服务启动函数
func startHTTPServer(host string, port int, debug bool) {
	// 直接调用main/http包中的HandleConnection函数
	mainhttp.HandleConnection(host, port, debug)
}

// SilicoID HTTP服务启动函数
func startSilicoidHTTPServer(host string, port int, debug bool) {
	// 直接调用silicoid/http包中的SilicoidHandleConnection函数
	silicoidhttp.SilicoidHandleConnection(host, port, debug)
}

// WebSocket服务启动函数
func startWebSocketServer(host string, port int, debug bool) {
	// 直接调用main/websocket包中的HandleConnection函数
	mainwebsocket.HandleConnection(host, port, debug)
}

// SilicoID WebSocket服务启动函数
/*func startSilicoidWebSocketServer(host string, port int, debug bool) {
	// 直接调用silicoid/websocket包中的HandleConnection函数
	silicoidwebsocket.HandleConnection(host, port, debug)
}*/

// MCP HTTP服务启动函数
func startMCPHTTPServer(host string, port int, debug bool) {
	// 直接调用modelcontextprotocol/http包中的HandleConnection函数
	mcphttp.HandleConnection(host, port, debug)
}

// 根据端口终止进程
func killProcessByPort(port int) {
	logger.Printf("尝试终止占用端口 %d 的进程", port)

	// 查找占用端口的进程 - 尝试多种方法
	var output []byte
	var err error

	// 方法1: 使用lsof
	cmd := exec.Command("lsof", "-t", "-i:"+strconv.Itoa(port))
	output, err = cmd.Output()
	if err != nil {
		// 方法2: 使用netstat + grep + awk
		cmd = exec.Command("sh", "-c", fmt.Sprintf("netstat -tulpn 2>/dev/null | grep :%d | awk '{print $7}' | cut -d'/' -f1 | grep -E '^[0-9]+$'", port))
		output, err = cmd.Output()
		if err != nil {
			// 方法3: 使用ss命令
			cmd = exec.Command("sh", "-c", fmt.Sprintf("ss -tulpn 2>/dev/null | grep :%d | awk '{print $7}' | sed 's/.*pid=//' | sed 's/,.*//' | grep -E '^[0-9]+$'", port))
			output, err = cmd.Output()
			if err != nil {
				logger.Printf("未找到占用端口 %d 的进程: %v", port, err)
				return
			}
		}
	}
	
	// 获取PID列表
	pidsStr := strings.TrimSpace(string(output))
	if pidsStr == "" {
		logger.Printf("未找到占用端口 %d 的进程", port)
		return
	}
	
	pidStrings := strings.Split(pidsStr, "\n")
	for _, pidStr := range pidStrings {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			logger.Printf("解析PID失败: %v", err)
			continue
		}
		
		// 使用kill命令强制终止进程
		killCmd := exec.Command("kill", "-9", strconv.Itoa(pid))
		err = killCmd.Run()
		if err != nil {
			logger.Printf("终止进程 %d 失败: %v", pid, err)
		} else {
			logger.Printf("成功终止进程 %d", pid)
			// 等待一会儿让端口释放
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func main() {
	// 确保工作目录正确：切换到程序所在目录或项目根目录
	// 这在开机自动启动时非常重要，因为systemd的工作目录可能不是项目根目录
	if err := ensureWorkingDirectory(); err != nil {
		fmt.Printf("警告: 无法设置工作目录: %v\n", err)
		// 继续执行，因为可能使用绝对路径
	}
	
	// 解析命令行参数
	httpHost := flag.String("httpHost", "0.0.0.0", "HTTP服务主机地址")
	httpPort := flag.Int("httpPort", 20717, "HTTP服务端口")
	websocketHost := flag.String("websocketHost", "0.0.0.0", "WebSocket服务主机地址")
	websocketPort := flag.Int("websocketPort", 20718, "WebSocket服务端口")
	silicoidHttpHost := flag.String("silicoidHttpHost", "0.0.0.0", "SilicoID HTTP服务主机地址")
	silicoidHttpPort := flag.Int("silicoidHttpPort", 30717, "SilicoID HTTP服务端口")
	/*silicoidWsHost := flag.String("silicoidWsHost", "0.0.0.0", "SilicoID WebSocket服务主机地址")
	silicoidWsPort := flag.Int("silicoidWsPort", 30718, "SilicoID WebSocket服务端口")*/
	mcpHttpHost := flag.String("mcpHttpHost", "0.0.0.0", "MCP HTTP服务主机地址")
	mcpHttpPort := flag.Int("mcpHttpPort", 40717, "MCP HTTP服务端口")
	debug := flag.Bool("debug", false, "是否启用调试模式")
	logLevel := flag.String("logLevel", "INFO", "日志级别 (DEBUG, INFO, WARNING, ERROR, CRITICAL)")
	httpOnly := flag.Bool("httpOnly", false, "仅启动HTTP服务")
	websocketOnly := flag.Bool("websocketOnly", false, "仅启动WebSocket服务")
	silicoidHttpOnly := flag.Bool("silicoidHttpOnly", false, "仅启动SilicoID HTTP服务")
	/*silicoidWsOnly := flag.Bool("silicoidWsOnly", false, "仅启动SilicoID WebSocket服务")*/
	mcpHttpOnly := flag.Bool("mcpHttpOnly", false, "仅启动MCP HTTP服务")
	
	flag.Parse()
	
	// 设置日志
	setupLogging(*logLevel)
	
	logger.Println("正在启动数字奇点后端服务...")
	
	// 根据命令行参数决定启动哪些服务
	startHTTP := true
	startWebSocket := true
	startSilicoIDHttp := true
	/*startSilicoIDWs := true*/
	startMCPHttp := true

	// 使用switch-case代替if-elif结构
	switch {
	case *httpOnly:
		startHTTP = true
		startWebSocket = false
		startSilicoIDHttp = false
		/*startSilicoIDWs = false*/
		startMCPHttp = false
		killProcessByPort(*httpPort)
	case *websocketOnly:
		startHTTP = false
		startWebSocket = true
		startSilicoIDHttp = false
		/*startSilicoIDWs = false*/
		startMCPHttp = false
		killProcessByPort(*websocketPort)
	case *silicoidHttpOnly:
		startHTTP = false
		startWebSocket = false
		startSilicoIDHttp = true
		/*startSilicoIDWs = false*/
		startMCPHttp = false
		killProcessByPort(*silicoidHttpPort)
	/*case *silicoidWsOnly:
		startHTTP = false
		startWebSocket = false
		startSilicoIDHttp = false
		startSilicoIDWs = true
		startMCPHttp = false
		killProcessByPort(*silicoidWsPort)*/
	case *mcpHttpOnly:
		startHTTP = false
		startWebSocket = false
		startSilicoIDHttp = false
		/*startSilicoIDWs = false*/
		startMCPHttp = true
		killProcessByPort(*mcpHttpPort)
	default:
		// 启动所有服务
		killProcessByPort(*httpPort)
		killProcessByPort(*websocketPort)
		killProcessByPort(*silicoidHttpPort)
		/*killProcessByPort(*silicoidWsPort)*/
		killProcessByPort(*mcpHttpPort)
	}
	
	// 使用WaitGroup等待所有服务
	var wg sync.WaitGroup
	
	// 启动HTTP服务
	if startHTTP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 额外等待一段时间，确保端口完全释放
			time.Sleep(300 * time.Millisecond)
			logger.Printf("正在启动HTTP服务，监听地址: %s:%d", *httpHost, *httpPort)
			startHTTPServer(*httpHost, *httpPort, *debug)
		}()
	}
	
	// 启动WebSocket服务
	if startWebSocket {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 额外等待一段时间，确保端口完全释放
			time.Sleep(500 * time.Millisecond)
			logger.Printf("正在启动WebSocket服务，监听地址: %s:%d", *websocketHost, *websocketPort)
			startWebSocketServer(*websocketHost, *websocketPort, *debug)
		}()
	}
	
	// 启动SilicoID HTTP服务
	if startSilicoIDHttp {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Printf("正在启动SilicoID HTTP服务，监听地址: %s:%d", *silicoidHttpHost, *silicoidHttpPort)
			startSilicoidHTTPServer(*silicoidHttpHost, *silicoidHttpPort, *debug)
		}()
	}
	
	// 启动SilicoID WebSocket服务
	/*if startSilicoIDWs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Printf("正在启动SilicoID WebSocket服务，监听地址: %s:%d", *silicoidWsHost, *silicoidWsPort)
			startSilicoidWebSocketServer(*silicoidWsHost, *silicoidWsPort, *debug)
		}()
	}*/

	// 启动MCP HTTP服务
	if startMCPHttp {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Printf("正在启动MCP HTTP服务，监听地址: %s:%d", *mcpHttpHost, *mcpHttpPort)
			startMCPHTTPServer(*mcpHttpHost, *mcpHttpPort, *debug)
		}()
	}
	
	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// 等待信号或所有服务完成
	go func() {
		sig := <-sigChan
		logger.Printf("接收到信号: %v, 正在关闭服务...", sig)
		// 这里可以实现优雅关闭
		os.Exit(0)
	}()
	
	// 等待所有服务
	wg.Wait()
	logger.Println("服务已关闭")
} 