# 项目概述

Poke 是 Docker 与 Docker Compose 可视化管理工具。前端使用 Vue 3、TypeScript、Vite、Pinia 和 Vue Router，后端使用 Go、Echo、Docker SDK 与 Gorilla WebSocket。

- 架构说明：`docs/architecture.md`。
- 部署说明：`docs/deployment.md`。
- 开发说明：`docs/development.md`。

# 开发环境

- 使用 `npm ci` 安装前端依赖；Vite 开发端口为 5173，代理后端 3001。
- 后端开发命令为在 `server` 目录执行 `PORT=3001 go run main.go`；Web 默认端口为 80。
- 本机管理开发启动需显式设置 `POKE_MODE=standalone PORT=3001 go run .`（在 `server` 目录）；前端使用 `npm run dev -- --port 5173 --strictPort`，防止端口冲突时自动切换而导致代理或来源校验不一致。
- 查看 Controller 无节点界面时，在 `server` 目录以 `POKE_MODE=controller POKE_LOCAL_ENABLED=false PORT=3001 go run .` 启动；已有节点不自动清空。切换模式会重启后端并使旧会话失效，管理员密码可复用。
- `POKE_MODE` 支持 `standalone`、`controller`、`agent`。Agent 不启动 HTTP 服务；Controller 默认不依赖本机 Docker。
- 前端检查使用 `npm run typecheck` 和 `npm test`；后端检查使用 `go test -race ./...`、`go vet ./...` 和 `gofmt -l .`。
- 前端界面参照相邻 `v2way` 的深灰、蓝色强调与紧凑布局，保留 Vue 3；相邻项目仅只读参考，不复制其私有数据。内容区最大宽度 1600px，720px 以下使用抽屉导航，保留浅色主题。
- 公共设计变量位于 `src/style.css`，资源列表及详情样式分别位于 `src/styles/resources.css`、`src/styles/details.css`。列表搜索复用 `ResourceToolbar` 与 `useResourceFilter`，仅在本地过滤，不改变 API 或节点作用域。
- 页面级操作（包括资源刷新）统一传送至顶部导航栏右侧的 `#header-action`；内容标题区不放操作按钮，列表内保留搜索与计数，行级操作仍随资源展示。窄屏操作区允许换行。
- Modal 保留焦点约束、Escape 关闭和关闭后焦点恢复；Select 使用传送到 body 的视口定位列表，支持方向键、Home/End、Enter 和 Escape，不恢复为不可聚焦的 div 触发器。
- 协议集成测试使用临时回环端口，不依赖真实 Docker；受限环境需要允许测试绑定端口。
- 不自动执行 Docker 构建、启动开发服务或安装系统软件；按明确任务授权操作。

# 测试账号

没有固定测试账号。首次启动 Web 服务时生成随机管理员密码，输出到服务日志。测试中的密码和域名均为构造数据。

# 常见问题

- 2026-11-17 已修复 Compose 文件符号链接越界及同站异源修改请求问题，依赖已更新；登录限速按用户要求暂不处理。
- 配置或节点 JSON 损坏时拒绝启动，应从备份修复，不得用空状态覆盖。
- Controller 和各 Agent 使用独立的持久化目录，角色目录的 `state.lock` 防止多实例并写。
- 默认配置目录为 `/poke`；无法创建时回退到当前工作目录下的 `data`。这些文件不得提交。
- 只读主目录环境中，可将 `GOCACHE`、`GOMODCACHE` 指向可写缓存目录。
- Controller 地址或节点 ID 改变时，需使用新的独立 Agent 数据目录注册，不能静默复用原身份。

# 其他备忘

- 镜像地址为 `ghcr.io/a1zaza/poke`；`.github/workflows/docker.yml` 原生构建 amd64/arm64 后合并发布。main 更新 latest，版本标签发布对应版本，PR 仅构建。工作流使用 GITHUB_TOKEN，关闭构建记录 artifact 上传，禁止改回跨任务 artifact 传递。

- 持久化仅使用 JSON，不引入数据库依赖。私有文件为 `0600`，采用同目录临时文件、文件同步和原子重命名。
- 每个节点使用独立身份；Controller 只保存凭据摘要。注册凭据有效期为 10 分钟，普通重启复用已保存身份。
- Agent 的 HTTP Transport 与 WebSocket Dialer 必须使用独立克隆的 TLS 配置；HTTP 可协商 HTTP/2，WebSocket 的 ALPN 仅允许 `http/1.1`。共享配置会使 HTTP 注册写入的 `h2` 污染控制和数据通道，触发 malformed HTTP response；保留 HTTP/2 注册后的双通道回归测试。
- 修改任务按任务 ID 去重；执行中断或取消后结果不明时，不得自动重放。
- 仓库凭据按节点授权和操作上下文隔离。Compose 使用临时私有 `DOCKER_CONFIG`，结束及 Agent 重启时清理。
- 所有项目文件操作必须经过 `server/api/project_path.go` 的路径边界校验；项目名最长 64 位，允许 ASCII 字母、数字、点、下划线和连字符，首字符为字母或数字。
- Compose 内容必须通过 `server/api/project_file.go` 访问：目录句柄限定边界、读取拒绝符号链接，写入使用独占临时文件和原子替换；不可恢复为直接按路径 `ReadFile` / `WriteFile`。
- `os.Root.OpenFile` 可能自行解析目录内符号链接；需要拒绝最终文件链接时，使用固定文件名和目录句柄调用 `openat(O_NOFOLLOW)`，保留目录内链接回归测试。
- 后端要求 Go 1.26.11 或更新补丁版本。Web API 使用标准库 `CrossOriginProtection`，包括登录、退出接口；反向代理和 Vite `/api` 代理必须保留原 Host、Origin 和 Sec-Fetch-Site。无浏览器来源头的 Agent 请求仍依靠节点认证。
- 管理员会话有效期 24 小时，签名密钥每次启动随机生成；HTTPS 入口需传递 `X-Forwarded-Proto: https` 或设置 `COOKIE_SECURE=true`。
- 密码使用 Argon2id；保留现有密码验证和迁移测试，避免清理文档时改变认证行为。
- 镜像采用多阶段构建，前端经 `embed_frontend` 标签编入 Go，不在工作区导出中间产物。
- 镜像打包测试可执行 `docker build --progress=plain -t poke:build-test .`；Dockerfile 内完成前端编译、嵌入资源测试、go vet 与 Go 编译。2026-11-17 已在 linux/amd64 构建成功，arm64 由工作流另行验证。
- 字体按用户要求与 `v2way` 一致：`Consolas, 'LXGW WenKai', monospace`。本地字体位于 `src/assets/fonts/`，通过 `@font-face` 和 `font-display: swap` 加载；表单与代码继承全局字体，xterm 在字体加载后初始化并采用同一字体栈。不要提交个人配置、真实凭据、运行数据或私有基础设施地址。
- 默认 Logo 的矢量源为 `public/logo.svg`，深蓝圆角底、蓝色立方容器、白色终端符号与薄荷绿连接节点构成标志；修改后同步导出透明背景的 512×512 PNG 到 `public/logo.png` 和 `public/default-logo.png`。页面与 favicon 继续使用动态 `/logo.png`，保留用户上传 Logo 的能力；README 使用 SVG。
- Vue 模板引用动态 `/logo.png` 时使用绑定属性（如 `:src="'/logo.png'"`），避免 Vite 将静态图片引用改写成 `/logo.png?import` 后被后端 Logo 代理返回 PNG，从而触发模块 MIME 错误。
- 提交前在 `docs/changelogs/YYYY-MM.md` 记录带时间戳的变更；未明确要求时不执行 Git 操作。
