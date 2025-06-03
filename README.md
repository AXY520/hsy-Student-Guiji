# 学生动态轨迹管理系统

一个基于Go + Gin框架的学生动态轨迹管理系统，集成高德地图API，支持标记点管理、图片上传、访问统计等功能。

## 🚀 功能特性

- **地图集成**: 集成高德地图API，支持2D地图和卫星图切换
- **标记点管理**: 创建、编辑、删除地图标记点
- **图片上传**: 支持为标记点上传多张图片
- **数据统计**: 记录用户访问和操作行为
- **响应式设计**: 支持PC和移动端访问
- **数据库优化**: 自动创建索引、视图和触发器

## 🛠️ 技术栈

### 后端
- **Go 1.20+**: 主要编程语言
- **Gin**: Web框架
- **SQLite**: 数据库
- **Zap**: 日志库
- **Lumberjack**: 日志轮转

### 前端
- **Vue.js 2**: 前端框架
- **Element UI**: UI组件库
- **高德地图API**: 地图服务
- **Axios**: HTTP客户端

## 📦 项目结构

```
├── main.go                 # 主程序入口
├── config.yaml            # 配置文件
├── go.mod                 # Go模块文件
├── Dockerfile             # Docker构建文件
├── docker-compose.yml     # Docker编排文件
├── pkg/                   # 核心包
│   ├── config/           # 配置管理
│   └── logger/           # 日志管理
├── static/               # 静态文件
│   ├── index.html        # 管理界面
│   ├── login.html        # 登录页面
│   ├── view.html         # 查看页面
│   └── components/       # 前端组件
├── uploads/              # 上传文件目录
└── logs/                 # 日志文件目录
```

## 🚀 快速开始

### 环境要求

- Go 1.20+
- SQLite3
- 高德地图API密钥

### 本地开发

1. **克隆项目**
```bash
git clone <repository-url>
cd hsy-Student-Guiji
```

2. **安装依赖**
```bash
go mod tidy
```

3. **设置环境变量**
```bash
export AMAP_API_KEY="your_amap_api_key"
```

4. **运行项目**
```bash
go run main.go
```

5. **访问应用**
- 管理界面: http://localhost:8080/admin
- 查看界面: http://localhost:8080/view
- 登录页面: http://localhost:8080/login

### Docker部署

1. **构建镜像**
```bash
docker build -t student-tracker .
```

2. **运行容器**
```bash
docker run -d \
  -p 8080:8080 \
  -e AMAP_API_KEY="your_amap_api_key" \
  -v $(pwd)/uploads:/app/uploads \
  -v $(pwd)/logs:/app/logs \
  student-tracker
```

### Docker Compose部署

```bash
# 设置环境变量
echo "AMAP_API_KEY=your_amap_api_key" > .env

# 启动服务
docker-compose up -d
```

## ⚙️ 配置说明

主要配置项在 `config.yaml` 文件中：

```yaml
server:
  port: 8080                    # 服务端口
  host: "0.0.0.0"              # 监听地址
  upload_dir: "./uploads"       # 上传目录
  max_file_size: 10485760      # 最大文件大小(10MB)

map:
  api_key: "${AMAP_API_KEY}"   # 高德地图API密钥
  default_center:
    latitude: 30.454407        # 默认中心纬度
    longitude: 114.390521      # 默认中心经度
  default_zoom: 18             # 默认缩放级别

database:
  path: "./markers.db"         # 数据库文件路径
  max_open_conns: 10          # 最大连接数
  max_idle_conns: 5           # 最大空闲连接数

security:
  allowed_origins:            # 允许的跨域来源
    - "http://localhost:8080"
  ip_whitelist: []            # IP白名单(空表示允许所有)

rate_limit:
  requests_per_second: 10     # 每秒请求限制
  burst: 20                   # 突发请求限制
```

## 🔧 API接口

### 标记点管理
- `GET /api/markers` - 获取所有标记点
- `POST /api/markers` - 创建标记点
- `PUT /api/markers/:id` - 更新标记点
- `DELETE /api/markers/:id` - 删除标记点

### 图片管理
- `POST /api/markers/:id/images` - 上传图片
- `DELETE /api/markers/:id/images/:filename` - 删除图片

### 系统监控
- `GET /api/health` - 健康检查
- `GET /api/health/ready` - 就绪检查
- `GET /api/health/live` - 存活检查

### 访问统计
- `GET /api/visits` - 获取访问统计

## 🧪 测试

运行单元测试：
```bash
go test ./...
```

运行特定包的测试：
```bash
go test ./pkg/handlers
```

## 📊 监控和日志

### 健康检查
系统提供多个健康检查端点：
- `/api/health` - 完整健康检查
- `/api/health/ready` - 就绪检查
- `/api/health/live` - 存活检查

### 日志管理
- 日志文件位置: `./logs/app.log`
- 支持日志轮转和压缩
- 结构化JSON格式日志

## 🔒 安全特性

- **输入验证**: 严格的参数验证和类型检查
- **文件上传安全**: 文件类型和大小限制
- **SQL注入防护**: 使用参数化查询
- **XSS防护**: 安全响应头设置
- **限流保护**: API请求频率限制
- **路径遍历防护**: 文件路径安全检查

## 🚀 性能优化

- **数据库索引**: 关键字段建立索引
- **连接池**: 数据库连接池管理
- **静态文件缓存**: 设置适当的缓存头
- **压缩**: 启用Gzip压缩
- **健康检查**: 系统状态监控

## 📝 开发指南

### 添加新功能
1. 在 `pkg/models/` 中定义数据模型
2. 在 `pkg/handlers/` 中实现HTTP处理器
3. 在 `main.go` 中注册路由
4. 编写单元测试

### 代码规范
- 遵循Go官方代码规范
- 使用有意义的变量和函数名
- 添加适当的注释和文档
- 编写单元测试

## 🤝 贡献指南

1. Fork项目
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建Pull Request

## 📄 许可证

本项目采用MIT许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🆘 问题反馈

如果您遇到任何问题或有改进建议，请：
1. 查看现有的Issues
2. 创建新的Issue描述问题
3. 提供详细的错误信息和复现步骤

## 📞 联系方式

- 项目维护者: [您的姓名]
- 邮箱: [您的邮箱]
- 项目地址: [项目仓库地址]
