cybersecurity/
├── network/           # 网络相关工具
│   ├── nmap.go        # 端口扫描
│   ├── rustscan.go    # 超快速端口扫描
│   ├── wireshark.go   # 网络协议分析器
│   └── aircrack_ng.go # WiFi安全审计套件
├── web/               # Web应用安全
│   └── ffuf.go        # 快速Web模糊测试
├── vulnerability/     # 漏洞扫描与利用
│   ├── nuclei.go      # 快速漏洞扫描器
│   └── metasploit.go  # 渗透测试框架
├── authentication/    # 认证与密码
│   └── hashcat.go     # GPU加速密码恢复
├── reverse_engineering/ # 逆向工程
│   └── ghidra.go      # NSA逆向工程套件
├── forensics/         # 取证分析
│   └── volatility3.go # 内存取证框架
├── container/         # 容器安全
│   └── trivy.go       # 容器漏洞扫描
└── tools.md           # 工具说明文档