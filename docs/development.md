# 开发说明

## 环境

- Node.js 24，与 Dockerfile 前端构建阶段保持一致。
- Go 1.26，与 Dockerfile 后端构建阶段保持一致。
- Docker 与 Compose 插件用于实际资源管理；单元测试不需要真实 Docker。

## 开发与检查

前端：

```bash
npm ci
npm run dev
```

后端在另一个终端运行：

```bash
cd server
PORT=3001 go run main.go
```

默认使用本机管理模式。访问 `http://localhost:5173`，开发代理连接后端 `3001`。Controller/Agent 环境变量见 [部署说明](deployment.md)。

检查命令：

```bash
npm run typecheck
npm test
```

```bash
cd server
gofmt -l .
go vet ./...
go test -race ./...
```

`gofmt -l` 应无输出。竞态检查需要本机 C 工具链；协议测试会监听临时回环端口，需要允许本地端口绑定。测试使用临时目录和模拟 Docker 响应，不要求启动开发服务或真实 Docker。

主目录只读时，可为 Go 命令设置可写的 `GOCACHE`、`GOMODCACHE` 和 `GOTMPDIR`。

## 镜像构建

根目录 Dockerfile 依次执行前端构建、Go 编译和运行镜像组装。`dist` 只存在于构建阶段，使用 `embed_frontend` 构建标签编入 Go 二进制。最终镜像包含服务和业务所需的 Docker CLI/Compose，Web 端口为 80。

构建示例由开发者按需执行：

```bash
docker build -t poke:local .
```

需要多架构发布时，使用 Buildx 并设置自己的镜像仓库地址：

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t registry.example.com/poke:VERSION --push .
```

构建阶段会执行真实嵌入资源的 Go 测试和静态检查。普通 `go test` 不依赖前端产物。无需预先生成工作区二进制或前端打包目录。

## 文件维护

- 部署示例使用通用占位域名与镜像地址，不提交个人基础设施配置。
- `.env`、运行数据、注册凭据、证书私钥、编辑器及本地工具配置不进入版本控制。
- 保留项目 Logo 和必要静态资源，界面使用系统字体。
- 第三方包名、来源链接和必要的许可证说明属于依赖信息，不作为个人数据删除。
- 提交前在 `docs/changelogs/YYYY-MM.md` 记录带时间戳的变更；不将临时调查、会话记录或过时方案作为当前文档。
