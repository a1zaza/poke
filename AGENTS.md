# 项目概述

Poke 是 Docker 与 Docker Compose 可视化管理工具。前端使用 Vue 3、TypeScript、Vite、Pinia 和 Vue Router，后端使用 Go、Echo、Docker SDK 与 Gorilla WebSocket。

- 架构说明：`docs/architecture.md`。
- 部署说明：`docs/deployment.md`。
- 开发说明：`docs/development.md`。

# 开发环境

- 使用 `npm ci` 安装前端依赖；Vite 开发端口为 5173，代理后端 3001。
- 后端开发命令为在 `server` 目录执行 `PORT=3001 go run main.go`；Web 默认端口为 80。
- `POKE_MODE` 支持 `standalone`、`controller`、`agent`。Agent 不启动 HTTP 服务；Controller 默认不依赖本机 Docker。
- 前端检查使用 `npm run typecheck` 和 `npm test`；后端检查使用 `go test -race ./...`、`go vet ./...` 和 `gofmt -l .`。
- 协议集成测试使用临时回环端口，不依赖真实 Docker；受限环境需要允许测试绑定端口。
- 不自动执行 Docker 构建、启动开发服务或安装系统软件；按明确任务授权操作。

# 测试账号

没有固定测试账号。首次启动 Web 服务时生成随机管理员密码，输出到服务日志。测试中的密码和域名均为构造数据。

# 常见问题

- 配置或节点 JSON 损坏时拒绝启动，应从备份修复，不得用空状态覆盖。
- Controller 和各 Agent 使用独立的持久化目录，角色目录的 `state.lock` 防止多实例并写。
- 默认配置目录为 `/poke`；无法创建时回退到当前工作目录下的 `data`。这些文件不得提交。
- 只读主目录环境中，可将 `GOCACHE`、`GOMODCACHE` 指向可写缓存目录。
- Controller 地址或节点 ID 改变时，需使用新的独立 Agent 数据目录注册，不能静默复用原身份。

# 其他备忘

- 持久化仅使用 JSON，不引入数据库依赖。私有文件为 `0600`，采用同目录临时文件、文件同步和原子重命名。
- 每个节点使用独立身份；Controller 只保存凭据摘要。注册凭据有效期为 10 分钟，普通重启复用已保存身份。
- 修改任务按任务 ID 去重；执行中断或取消后结果不明时，不得自动重放。
- 仓库凭据按节点授权和操作上下文隔离。Compose 使用临时私有 `DOCKER_CONFIG`，结束及 Agent 重启时清理。
- 所有项目文件操作必须经过 `server/api/project_path.go` 的路径边界校验；项目名最长 64 位，允许 ASCII 字母、数字、点、下划线和连字符，首字符为字母或数字。
- 管理员会话有效期 24 小时，签名密钥每次启动随机生成；HTTPS 入口需传递 `X-Forwarded-Proto: https` 或设置 `COOKIE_SECURE=true`。
- 密码使用 Argon2id；保留现有密码验证和迁移测试，避免清理文档时改变认证行为。
- 镜像采用多阶段构建，前端经 `embed_frontend` 标签编入 Go，不在工作区导出中间产物。
- 仓库使用系统字体，不分发本地字体文件。不要提交个人配置、真实凭据、运行数据或私有基础设施地址。
- 提交前在 `docs/changelogs/YYYY-MM.md` 记录带时间戳的变更；未明确要求时不执行 Git 操作。
