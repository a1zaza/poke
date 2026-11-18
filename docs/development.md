# 开发说明

## 环境

- Node.js 24，与 Dockerfile 前端构建阶段保持一致。
- Go 1.26.11 或更新补丁版本，Dockerfile 后端构建阶段使用 1.26.11。
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

API 开发代理必须保留原始 Host，供后端跨源请求防护校验；不要将 `/api` 的 `changeOrigin` 改为 `true`。开发服务器仅用于可信网络。

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

## 前端界面

控制台采用紧凑的侧栏、面包屑和页面标题布局，支持深浅主题；手机端使用抽屉导航。共享颜色、边框和控件变量集中在 `src/style.css`，列表和详情分别复用 `src/styles/resources.css`、`src/styles/details.css`，避免逐页复制样式。

资源页通过 `ResourceToolbar` 与 `useResourceFilter` 提供本地搜索、数量和刷新入口；搜索不改变节点选择及后端请求。系统管理使用三个独立一级菜单：基本设置（`/settings`）、镜像仓库（`/registries`）、节点与任务（`/nodes`，仅 Controller 显示）。各页仅保存自身配置，加载失败时禁止保存。弹窗和下拉框需保留键盘、焦点及窄屏行为，危险操作使用确认弹窗。

2026-11-17 前端重构已通过类型检查和 16 项单元测试，并以实际 Vue 组件、模拟 API 完成桌面、手机和短横屏浏览器验证；该验证不代表真实 Docker 或多节点联调。

## 镜像构建

根目录 Dockerfile 依次执行前端构建、Go 编译和运行镜像组装。`dist` 只存在于构建阶段，使用 `embed_frontend` 构建标签编入 Go 二进制。最终镜像包含服务和业务所需的 Docker CLI/Compose，Web 端口为 80。

构建示例由开发者按需执行：

```bash
docker build -t ghcr.io/a1zaza/poke:local .
```

需要手动发布多架构镜像时，先登录 GHCR，再使用 Buildx：

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/a1zaza/poke:VERSION --push .
```

构建阶段会执行真实嵌入资源的 Go 测试和静态检查。普通 `go test` 不依赖前端产物。无需预先生成工作区二进制或前端打包目录。

## 自动发布

工作流位于 `.github/workflows/docker.yml`，镜像发布到 `ghcr.io/a1zaza/poke`。原生 amd64、arm64 runner 分别构建，全部成功后合并为多架构索引；Controller、Agent 和单机模式共用此镜像。

| 触发方式 | 发布结果 |
| --- | --- |
| 推送 `main` | `latest` 和 `sha-<完整提交 SHA>` |
| 推送 `v1.2.3` | `v1.2.3`、`1.2.3`、`1.2` 和 SHA 标签 |
| 推送 `v1.2.3-rc.1` | 原始预发布标签、对应完整预发布版本和 SHA 标签，不更新 `latest` 或 `1.2` |
| 提交 PR | 只构建验证，不登录或推送 GHCR |
| 手动触发 | 在 `main` 或 `v*` 标签上发布，其他分支只验证构建 |

`latest` 跟随 `main`，版本标签发布不覆盖 `latest`。架构构建使用带运行 ID 的 `build-*` 镜像标签中转，防止合并时引用其他运行的产物，也支持只重跑失败的架构任务。构建摘要和构建记录 artifact 上传均已关闭，不使用 artifact 上传/下载步骤；缓存通过 BuildKit 的 GitHub Actions 缓存后端管理。

发布使用仓库自带的 `GITHUB_TOKEN` 和 `packages: write` 权限，无需额外的个人访问令牌。仓库需允许 Actions 运行并写入 Packages。首次发布后，如需匿名拉取，请在 GitHub Packages 中将对应包设为公开。相关机制见 [GitHub 镜像发布文档](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images)；关闭构建记录上传见 [构建 action 配置](https://github.com/docker/build-push-action#environment-variables)。

Fork 后工作流根据仓库名自动生成 GHCR 地址，部署模板中的默认镜像仍需改为自己的包地址。

## 文件维护

- 部署示例使用公开的项目镜像地址和通用占位域名，不提交个人基础设施配置。
- `.env`、运行数据、注册凭据、证书私钥、编辑器及本地工具配置不进入版本控制。
- 保留项目 Logo 和必要静态资源；界面字体与 v2way 一致，使用 `src/assets/fonts/` 下的 Consolas 与霞鹜文楷，通过本地 Web 字体加载，终端采用同一字体栈。
- 第三方包名、来源链接和必要的许可证说明属于依赖信息，不作为个人数据删除。
- 提交前在 `docs/changelogs/YYYY-MM.md` 记录带时间戳的变更；不将临时调查、会话记录或过时方案作为当前文档。
