## Digital Singularity 服务器部署手册（Ubuntu）

本文档说明如何在 **全新 Ubuntu 服务器** 上部署 Digital Singularity 后端服务，包括：

- **创建运行账号 `ubuntu:ubuntu` 并配置 `/program` 目录**
- **安装并配置 MySQL 和 Redis**
- **安装 Go 语言环境并配置国内镜像（GOPROXY）**
- **部署后端代码并以 systemd 服务方式运行**

> 所有命令在无特殊说明时，均在 **SSH 登录后的终端** 中执行，需要具备 `sudo` 权限。

---

## 一、创建运行用户 ubuntu:ubuntu 并准备目录

### 1. 创建 ubuntu 用户（如已有可跳过）

```bash
sudo adduser ubuntu
# 按提示设置密码（可以设为 ubuntu，仅用于测试环境）

# 可选：将 ubuntu 加入 sudo 组，便于日常运维
sudo usermod -aG sudo ubuntu
```

### 2. 创建程序目录 /program 并授权给 ubuntu:ubuntu

```bash
sudo mkdir -p /program
sudo chown -R ubuntu:ubuntu /program
```

### 3.（可选）将主机名改为 ubuntu（影响终端提示）

```bash
sudo hostnamectl set-hostname ubuntu
```

然后编辑 `/etc/hosts`，保证里面有一行主机名为 `ubuntu`，例如：

```text
127.0.0.1   localhost
127.0.1.1   ubuntu
```

保存后重新登录终端，提示符会变为类似：

```bash
ubuntu@ubuntu:~$
```

---

## 二、安装基础依赖（MySQL + Redis）

### 1. 更新软件源

```bash
sudo apt-get update
sudo apt-get upgrade -y
```

### 2. 安装 MySQL Server

```bash
sudo apt-get install -y mysql-server
```

> 若系统版本较新，可执行 `sudo mysql_secure_installation` 按提示设置 root 密码和安全选项。

#### 2.1 创建数据库和导入表结构

根据 `digitalsingularity.service.example` 中的配置，默认：

- **数据库主机**：`localhost`
- **端口**：`3306`
- **用户**：`root`
- **密码**：`XXXXXXXXXXXXXXXXXXXXXXXXXXX`
- **数据库名**：`XXXXXXXXXXXXXXXXXXXXXXXXXXX`

请根据实际情况选择：

1）**方案 A：让 MySQL root 使用密码 `XXXXXXXXXXXXXXXXXXXXXXXXXXX`（与示例一致）**

```bash
sudo mysql

-- 设置 root 账号密码（仅示例，按需调整）
ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY 'XXXXXXXXXXXXXXXXXXXXXXXXXXX';
FLUSH PRIVILEGES;
EXIT;
```

2）**方案 B：使用你自己的数据库账号/密码**  
则需要同时修改后文中 systemd 服务文件里的以下环境变量：

- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`

#### 2.2 允许外部访问（bind `0.0.0.0`，按需开启）

> 仅在需要外部机器访问 MySQL 时配置；如果只在本机访问，可以跳过这一小节。

编辑配置文件（不同版本路径可能略有差异，一般为以下之一）：

```bash
sudo nano /etc/mysql/mysql.conf.d/mysqld.cnf
# 如果不存在，再尝试：
# sudo nano /etc/mysql/mariadb.conf.d/50-server.cnf
```

找到类似下面这一行：

```text
bind-address = 127.0.0.1
```

修改为（允许所有地址访问）：

```text
bind-address = 0.0.0.0
```

保存后重启 MySQL：

```bash
sudo systemctl restart mysql
sudo systemctl status mysql
```

> **强烈建议** 同时通过防火墙/安全组限制访问源 IP，只开放给可信的应用服务器或管理机器。

#### 2.3 允许 root 通过 Navicat 等客户端远程连接（按需开启）

> 仅在需要用 Navicat/其他图形工具从外部机器管理数据库时配置；生产环境请务必限制来源 IP。

1）进入 MySQL：

```bash
sudo mysql
```

2）为 `root@'%'` 设置密码和权限（示例密码仍为 `XXXXXXXXXXXXXXXXXXXXXXXXXXX`，按需修改）：

```sql
CREATE USER IF NOT EXISTS 'root'@'%' IDENTIFIED WITH mysql_native_password BY 'XXXXXXXXXXXXXXXXXXXXXXXXXXX';
ALTER USER 'root'@'%' IDENTIFIED WITH mysql_native_password BY 'XXXXXXXXXXXXXXXXXXXXXXXXXXX';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' WITH GRANT OPTION;
FLUSH PRIVILEGES;
```

3）退出 MySQL：

```sql
EXIT;
```

4）如果服务器启用了 ufw 防火墙，还需放行 3306 端口（按需限制来源 IP）：

```bash
sudo ufw allow 3306/tcp
sudo ufw reload
```

> 在 Navicat 中连接时，主机填服务器公网 IP（或内网 IP），端口 3306，用户名 `root`，密码 `XXXXXXXXXXXXXXXXXXXXXXXXXXX`。

#### 2.4 创建数据库并导入 SQL

```bash
mysql -uroot -p'XXXXXXXXXXXXXXXXXXXXXXXXXXX' -e "CREATE DATABASE IF NOT EXISTS security_check DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
```

项目中与数据库结构相关的 SQL 文件在：

- `backend/data_structure/security_check/`（安全检测相关）
- `backend/data_structure/common/` 等（如需其它模块）

你可以根据实际需要导入对应 SQL，例如：

```bash
cd /program/digitalsingularity/backend/data_structure

# 下面仅举例，请按实际需要选择要导入的 SQL 文件
mysql -uroot -p'XXXXXXXXXXXXXXXXXXXXXXXXXXX' security_check < security_check/001_init_tables.sql
mysql -uroot -p'XXXXXXXXXXXXXXXXXXXXXXXXXXX' security_check < security_check/002_xxx.sql
# ...
```

> 如果不确定要导入哪些 SQL，可将 `security_check` 目录下的 SQL 逐个按排序导入，或根据你的业务范围选择。

### 3. 安装 Redis

```bash
sudo apt-get install -y redis-server
```

#### 3.1 配置 Redis 监听地址和密码（示例：`0.0.0.0` + `XXXXXXXXXXXXXXXXXXXXXXXXXXX`）

先改好配置，再启动服务。

编辑配置文件：

```bash
sudo nano /etc/redis/redis.conf
```

Ctrl+w 搜索并修改 `bind` 和 `requirepass`（注意密码里的 `^` 不需要转义，直接写在配置里即可）：

```text
bind 0.0.0.0
requirepass XXXXXXXXXXXXXXXXXXXXXXXXXXX
```

保存退出。

> 开放 `bind 0.0.0.0` 后，请务必通过防火墙/安全组限制 Redis 端口（默认 6379）的来源 IP。

#### 3.2 启用开机自启并启动 Redis

```bash
sudo systemctl enable redis-server
sudo systemctl start redis-server
sudo systemctl status redis-server
```

使用 `redis-cli` 测试密码是否生效：

```bash
redis-cli
127.0.0.1:6379> AUTH "XXXXXXXXXXXXXXXXXXXXXXXXXXX"
OK
```

> 如果应用需要连接有密码的 Redis，请在应用配置中使用相同的密码（例如 `redis://:XXXXXXXXXXXXXXXXXXXXXXXXXXX@127.0.0.1:6379/0`）。

---

## 三、安装 Go 语言环境并配置国内镜像

### 1. 安装 Go

这里提供两种方式，二选一即可。

#### 方式 A：通过 apt 安装（命令简单）

```bash
sudo apt-get install -y golang
go version   # 确认安装成功
```

> 不同 Ubuntu 版本自带的 Go 版本可能偏旧，如果你有特定版本需求，可以使用方式 B。

#### 方式 B：使用官方二进制到 /usr/local（可控版本）

以 Go 1.22.x 为例，请根据需要替换为最新版本：

```bash
cd /tmp
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz

sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
```

将 Go 添加到 PATH（对当前用户），以 `ubuntu` 用户身份执行：

```bash
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc

go version   # 确认能正常输出版本
```

> `digitalsingularity.service.example` 中已经内置了 `PATH=/usr/local/go/bin:...`，如果使用方式 B 安装，确保 Go 实际安装在 `/usr/local/go` 即可。

### 2. 配置 Go 模块国内镜像（GOPROXY）

**方式一：全局 go env（开发/编译时生效）**

```bash
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.org
```

**方式二：通过 systemd 环境变量（运行服务时生效）**

示例服务文件 `digitalsingularity.service.example` 已包含：

```ini
Environment="GOPROXY=https://goproxy.cn,direct"
```

如果你只在运行时需要镜像，这一行保持不动即可。

---

## 四、部署 Digital Singularity 代码与可执行文件

### 1. 将项目代码放到 /program/digitalsingularity

以 `ubuntu` 用户执行：

```bash
cd /program
# 如果是从 git 拉取
git clone <你的仓库地址> digitalsingularity

# 或者将现有代码/打包文件上传到服务器后解压到 /program/digitalsingularity
```

确保目录结构类似：

```text
/program/digitalsingularity
  ├── backend/
  ├── digitalsingularity.service.example
  └── ...
```

然后再次确认权限：

```bash
sudo chown -R ubuntu:ubuntu /program/digitalsingularity
```

### 2. 编译后端可执行文件（如尚未编译）

> 编译命令需根据实际 `main` 函数所在包路径调整，以下为示例：

```bash
cd /program/digitalsingularity

# 示例：将可执行文件命名为 digitalsingularity，放在项目根目录
go build -o digitalsingularity ./backend/main
```

编译完成后，项目根目录应有可执行文件：

```bash
ls -lh /program/digitalsingularity/digitalsingularity
```

> 如果实际入口在其它目录（例如某个特定子模块），请按实际调整 `go build` 的路径，只要最终 `ExecStart` 对应的可执行文件存在即可。

---

## 五、配置并启动 systemd 服务

项目中提供了示例服务文件 `digitalsingularity.service.example`，内容关键段为：

```ini
[Service]
Type=simple
User=ubuntu
Group=ubuntu
WorkingDirectory=/program/digitalsingularity
ExecStart=/program/digitalsingularity/digitalsingularity
Environment="PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
Environment="GOPROXY=https://goproxy.cn,direct"

Environment="DIGITALSINGULARITY_LOG_DIR=/var/log/digitalsingularity_logs"
Environment="GIN_MODE=release"
Environment="DB_HOST=localhost"
Environment="DB_PORT=3306"
Environment="DB_USER=root"
Environment="DB_PASSWORD=XXXXXXXXXXXXXXXXXXXXXXXXXXX"
Environment="DB_NAME=security_check"
```

请根据你的环境检查并必要时修改：

- **User / Group**：确保存在 `ubuntu:ubuntu`
- **WorkingDirectory / ExecStart**：路径与实际部署一致
- **数据库相关环境变量** 与 MySQL 实际设置匹配

### 1. 拷贝服务文件到 systemd 目录

```bash
cd /program/digitalsingularity
sudo cp digitalsingularity.service.example /etc/systemd/system/digitalsingularity.service
```

如需编辑：

```bash
sudo nano /etc/systemd/system/digitalsingularity.service
```

### 2. 创建日志目录并授权

```bash
sudo mkdir -p /var/log/digitalsingularity_logs
sudo chown -R ubuntu:ubuntu /var/log/digitalsingularity_logs
```

### 3. 重新加载 systemd 并设置开机自启

```bash
sudo systemctl daemon-reload
sudo systemctl enable digitalsingularity.service
```

### 4. 启动服务并检查状态

```bash
sudo systemctl start digitalsingularity.service
sudo systemctl status digitalsingularity.service
```

如需要停止/重启：

```bash
sudo systemctl restart digitalsingularity.service
sudo systemctl stop digitalsingularity.service
```

---

## 六、日志查看与故障排查

### 1. 查看应用日志

根据示例服务文件，日志默认写入：

- 标准输出：`/var/log/digitalsingularity_logs/service.log`
- 错误输出：`/var/log/digitalsingularity_logs/service_error.log`

可以通过以下命令实时查看：

```bash
tail -f /var/log/digitalsingularity_logs/service.log
tail -f /var/log/digitalsingularity_logs/service_error.log
```

### 2. 使用 journalctl 查看 systemd 日志

```bash
sudo journalctl -u digitalsingularity.service -n 100 --no-pager
sudo journalctl -u digitalsingularity.service -f
```

### 3. 常见问题检查项

- **服务无法启动**
  - 使用 `systemctl status` 与 `journalctl` 查看详细错误信息
  - 检查 `ExecStart` 指向的可执行文件是否存在且可执行
  - 检查数据库连接配置是否正确（主机、端口、用户、密码、数据库名）
  - 确认 MySQL、Redis 服务已启动

- **权限问题**
  - 确认 `/program/digitalsingularity` 和 `/var/log/digitalsingularity_logs` 目录为 `ubuntu:ubuntu` 所有
  - 如有文件权限错误，可执行：

    ```bash
    sudo chown -R ubuntu:ubuntu /program/digitalsingularity
    sudo chown -R ubuntu:ubuntu /var/log/digitalsingularity_logs
    ```

---

## 七、快速检查服务是否正常工作

服务启动后，你可以按以下思路进行健康检查（具体接口以项目实际为准）：

- 使用 `ss` 或 `netstat` 检查监听端口：

```bash
sudo ss -tlnp | grep <你的服务端口>
```

- 使用 `curl` 访问健康检查或任意公开 HTTP 接口，例如：

```bash
curl http://127.0.0.1:<你的服务端口>/health
```

若能正常返回 JSON 或预期内容，则说明 Digital Singularity 后端服务已成功部署并运行。




