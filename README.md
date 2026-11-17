# Poke

Poke 是一个 Docker 与 Docker Compose 可视化管理工具，支持本机管理和 Agent 多节点管理。前端资源嵌入 Go 二进制，通过同一服务提供页面、API 和终端连接。

## 功能

- 容器、镜像、网络与数据卷管理。
- Compose 项目的创建、编辑、部署、启停及更新。
- 系统概览、容器日志、镜像更新检查和交互式终端。
- Agent 主动连接 Controller，支持 NAT 后的节点。
- 节点注册、仓库授权、身份撤销及远程任务状态查询。
- 配置、节点身份与任务记录使用 JSON 持久化。

## 运行模式

| 模式 | 用途 |
| --- | --- |
| `standalone` | 默认模式，管理本机 Docker |
| `controller` | 提供统一管理入口，接收 Agent 连接 |
| `agent` | 主动连接 Controller，在本机执行操作，无需开放入站端口 |

Controller 默认不管理本机 Docker；可通过 `POKE_LOCAL_ENABLED=true` 启用。

## 部署

默认镜像为 `ghcr.io/a1zaza/poke:latest`，支持 `linux/amd64` 和 `linux/arm64`。复制 `.env.example` 为本机 `.env`；可用 `POKE_IMAGE` 指定版本，例如 `ghcr.io/a1zaza/poke:1.0.0`。Controller 域名仍需按实际部署填写。

主节点使用 [compose.yaml](compose.yaml)：

```bash
docker compose -f compose.yaml up -d
```

Agent 在自己的主机上使用 [compose.agent.yaml](compose.agent.yaml)。先在 Controller 的系统设置中注册节点，再填写主节点 HTTPS 地址、节点 ID 和注册凭据文件：

```bash
docker compose -f compose.agent.yaml up -d
```

默认 Web 端口为 `80`。首次启动 Web 服务时，随机管理员密码会输出到服务日志。Agent 挂载 `/poke:/poke` 和 Docker socket，项目、身份、任务及注册材料统一保存在 `/poke` 下。

完整的环境变量、HTTPS 设置、目录布局和备份方法见 [部署说明](docs/deployment.md)。

## 镜像发布

[GitHub Actions 工作流](.github/workflows/docker.yml) 在推送 `main` 时更新 `latest`；推送 `v1.0.0` 这样的标签时发布对应版本。PR 仅验证构建。Controller、Agent 和本机模式共用同一镜像。

标签规则、GHCR 权限及手动发布说明见 [开发说明](docs/development.md#自动发布)。

## 本地开发

需要 Node.js 24、Go 和可访问的 Docker 守护进程。生产构建所用版本见 [Dockerfile](Dockerfile)。

安装前端依赖：

```bash
npm ci
```

后端和前端分别运行：

```bash
cd server
PORT=3001 go run main.go
```

```bash
npm run dev
```

访问 `http://localhost:5173`。Vite 将 API、终端和 Logo 请求代理到后端 `3001`。

开发检查、镜像构建和代码组织见 [开发说明](docs/development.md)。协议与数据模型见 [架构说明](docs/architecture.md)。

## 文件结构

```text
src/                     前端页面、组件和状态管理
public/                  默认图标与网页清单
server/api/              Docker、Compose 与管理接口
server/internal/cluster/ 节点协议、连接管理和 JSON 存储
server/pkg/              配置、认证、镜像等公共模块
server/webui/            嵌入式前端资源服务
compose.yaml             Controller 部署模板
compose.agent.yaml       Agent 部署模板
docs/                    架构、部署和开发说明
```

## 许可

仓库尚未附带 `LICENSE` 文件。
