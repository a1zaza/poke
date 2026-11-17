# Controller / Agent 部署

Agent 主动连接 Controller，节点身份、配置和任务以 JSON 持久化。

管理页面和 API 应通过同一个来源访问。反向代理必须保留浏览器访问的 Host（包括非默认端口），并保留 Origin、Sec-Fetch-Site 请求头；否则修改请求可能被跨源防护拒绝。不要通过信任任意来源来绕过此防护。

## 运行模式

| 环境变量 | 含义 |
| --- | --- |
| `POKE_MODE=standalone` | 默认模式，管理本机 Docker，提供页面与 API |
| `POKE_MODE=controller` | 统一管理入口，接收 Agent 连接，默认无需挂载 Docker socket |
| `POKE_MODE=agent` | 执行节点，仅主动出站连接，不监听 HTTP 端口 |
| `POKE_LOCAL_ENABLED=true` | Controller 同时管理本机 Docker；此时需要 Docker socket 与项目目录 |
| `PORT` | standalone/controller 的监听端口，默认 80 |
| `POKE_CONTROLLER_URL` | Agent 访问的 Controller HTTPS 地址，可包含部署子路径 |
| `POKE_NODE_ID` | 页面生成的稳定节点 ID |
| `POKE_ENROLLMENT_TOKEN_FILE` | 初次注册或重新注册时，一次性凭据所在文件 |
| `POKE_CA_FILE` | 可选，Agent 信任的私有 CA 文件 |
| `POKE_ALLOW_INSECURE_HTTP=true` | 仅隔离测试环境允许明文 HTTP；默认关闭 |

## 首次接入

管理员默认用户名为 `admin`。Web 首次初始化成功后，日志会打印一次用户名和随机密码；重启不会重复打印或重置账号。登录需要同时填写用户名和密码，可在“系统设置 → 基本设置”修改用户名。用户名为 1–64 位，以字母或数字开头，支持字母、数字、下划线、点、`@` 和连字符；已有配置缺少用户名时会补为 `admin` 并保留原密码。

仓库提供两份独立 Compose 文件：主节点使用根目录 `compose.yaml`，各从节点使用 `compose.agent.yaml`。两端分别复制 `.env.example` 为本机 `.env`，默认使用 `ghcr.io/a1zaza/poke:latest`；可将 `POKE_IMAGE` 设置为固定版本，例如 `ghcr.io/a1zaza/poke:1.0.0`。Controller 域名需替换为实际地址。

主节点配置镜像、端口及 HTTPS 入口后，由部署者执行：

```bash
docker compose -f compose.yaml up -d
```

从节点先准备宿主机 `/poke` 目录及 Docker socket，将注册凭据保存到 `/poke/enrollment-token`（权限 `0600`），填写本机 `.env` 中的 Controller 地址和节点 ID，再执行：

```bash
docker compose -f compose.agent.yaml up -d
```

两个文件不合并运行。Controller 默认仅管理远程节点，不挂载本机 Docker socket；Agent 无端口映射。Controller 使用独立命名数据卷，Agent 将宿主机 `/poke` 整体映射到容器 `/poke`，身份、任务、项目及注册材料统一保存在该目录。Agent 注册成功后，清空 `POKE_ENROLLMENT_TOKEN_FILE` 并重建该容器，再删除 `/poke/enrollment-token`。

`.env` 已加入 Git 忽略规则，Agent 数据和注册材料保存在仓库之外的宿主机 `/poke`。模板采用挂载短写语法，启动前应确认 `/poke` 和 Docker socket 存在；不能依赖目录挂载自动生成注册凭据文件。

1. 以 Controller 模式部署当前 Poke 镜像，持久化 `/poke`。容器监听 80，对外提供受信任的 HTTPS 地址。
2. HTTPS 入口需支持 `/agent/v1/connect`、`/agent/v1/stream` 和 `/ws/*` 的 WebSocket 升级，空闲连接超时应大于 60 秒。HTTPS 管理入口继续传递 `X-Forwarded-Proto: https` 或使用 `COOKIE_SECURE=true`。
3. 登录 Controller，进入“系统设置”。需要私有仓库时先保存仓库配置，再在“Agent 节点”输入名称、选择授权仓库并生成注册凭据。
4. 在 Agent 主机将凭据写入 `/poke/enrollment-token`，权限设为 `0600`，通过 `/poke:/poke` 一并提供给容器。不要把实际凭据写入镜像、仓库、URL 或日志。
5. 使用同一 Poke 镜像设置 `POKE_MODE=agent`、`POKE_CONTROLLER_URL`、`POKE_NODE_ID` 和 `POKE_ENROLLMENT_TOKEN_FILE`。挂载宿主机 `/poke:/poke` 和 Docker socket；项目自动位于该目录下的 `projects` 子目录，无需额外挂载。无需映射 Agent 端口。建议启用容器运行时 `init` 和失败重启策略。
6. Agent 完成注册、保存身份并主动连接后，页面显示“在线”。选择该节点即可管理 Docker 资源。
7. 注册成功后可移除注册凭据文件及 `POKE_ENROLLMENT_TOKEN_FILE`，保留 Agent 身份目录。初次注册响应丢失时，应保留原注册材料重试；程序会使用已经落盘的同一注册尝试，避免消耗第二个身份。

主节点和各 Agent 必须使用不同的数据卷。若更换 Controller 地址或节点 ID，先归档旧 Agent 数据，再使用新的独立数据目录完成注册；程序会拒绝将旧身份静默绑定到新配置。Agent 地址无需配置到 Controller，也不要求 Controller 能直接连接 Agent。若 Compose 项目使用相对路径 bind mount，Agent 内 `/poke/projects` 与 Docker 宿主机的项目路径需要保持一致，避免 Docker 将路径解析到另一位置。

Agent 与 Docker 的通信仍取决于 Docker socket 或 `DOCKER_HOST`；节点控制通道在线表示管理连接正常，Docker 操作失败会作为明确的接口或任务错误返回。

## 可选自定义配置

Compose 模板仅保留必要配置；按需在部署副本中增加以下内容，不需要全部启用。

| 需求 | 调整方式 |
| --- | --- |
| 修改主节点端口或绑定地址 | 修改 `ports`，例如 `127.0.0.1:8080:80`；容器端口保持 80 |
| 强制 HTTPS Cookie | Controller 的 `environment` 增加 `COOKIE_SECURE: "true"`；HTTP 初始化访问时不要启用 |
| 私有 CA | 将 CA 保存为宿主机 `/poke/ca.pem`，Agent 的 `environment` 增加 `POKE_CA_FILE: /poke/ca.pem` |
| 自定义时区 | 服务的 `environment` 增加 `TZ: Asia/Shanghai` |
| Rootless Docker | 将 Agent 的 socket 挂载源改为实际路径，容器目标仍为 `/var/run/docker.sock` |
| 日志轮转 | 服务下增加 `logging`，使用 `json-file`，设置 `max-size: "10m"`、`max-file: "3"` |
| 停止等待时间 | 服务下增加 `stop_grace_period`，建议 Controller 为 `15s`、Agent 为 `30s` |
| Controller 健康检查 | 服务下增加 `healthcheck`，在容器内请求 `http://127.0.0.1:80/health` |
| Controller 管理本机 Docker | 增加 `POKE_LOCAL_ENABLED: "true"`，并挂载 Docker socket 与 `/poke/projects` |

首次注册后清空 `.env` 中的 `POKE_ENROLLMENT_TOKEN_FILE`，重建 Agent 后可删除注册凭据文件。请保留 Controller 的数据卷，以及 Agent 宿主机 `/poke` 下的数据，以复用原有身份及任务状态。

## 身份和权限管理

- 注册凭据有效期 10 分钟，每次仅用于一个预创建节点；普通重启使用持久身份自动重连。
- 节点连接凭据彼此独立；Controller 只保存摘要，Agent 将秘密保存在私有身份文件中。
- 页面可调整某节点的仓库授权。保存后断开该节点当前连接，Agent 使用原身份重新连接；后续操作按新授权取凭据。
- “重新注册”立即使旧身份失效并生成新注册凭据。将新凭据交给原 Agent 并保留相同节点 ID；程序检测到新的注册材料后重新建立身份。新旧凭据不并行接受。
- “撤销节点”立即关闭该节点当前的控制、查询与终端连接，拒绝后续连接；已启动容器继续运行。已交给 Docker 的操作不能承诺回滚。

## 操作与任务

远程查询通过控制连接通知 Agent，再由 Agent 主动建立单独的数据连接返回 JSON。终端另用独立的双向数据连接；页面断开后不会重放终端输入。日志保持行数受限的查询方式。

远程修改先记录任务 ID，Agent 保存接收状态后执行。页面等待执行结果，也可在系统设置中查看最近 200 项任务。Agent 将修改操作串行处理，并限制等待队列；重复投递同一任务 ID 不会再次执行。

取消操作是尽力而为：尚未开始的任务可标记已取消；执行期间超时、取消或进程崩溃的任务标记“结果待确认”，需要检查实际 Docker 状态。浏览器关闭不会自动取消已经接收的修改任务。Controller 或 Agent 重启后会自动重新连接并补报已保存的终态结果，未确认副作用不会自动重放。

仓库秘密只随授权的相关操作临时发送，不写入任务记录；Compose 在执行前解析项目实际引用的镜像，将需要的仓库授权写入临时私有 `DOCKER_CONFIG`，结束后清理；Agent 在持有独占状态锁后，启动时还会清理该角色的遗留临时凭据。镜像 SDK 与 Compose 使用相同的操作级凭据范围。

当前 standalone/Controller 本机管理使用进程内 API，远程 Agent 修改使用任务接口。系统不提供跨节点调度或主节点自动故障切换。

## JSON 文件布局与备份

| 文件 | 内容 |
| --- | --- |
| `/poke/config.json` | Controller/standalone 的普通设置、管理员用户名与密码哈希 |
| `/poke/secrets.json` | 私有仓库用户名和密码 |
| `/poke/controller/nodes.json` | 节点信息、仓库授权、注册凭据摘要及独立身份摘要 |
| `/poke/controller/tasks/<ID>.json` | Controller 的任务状态与结果 |
| `/poke/agent/identity.json` | Agent 节点身份秘密、Controller 地址与注册尝试信息 |
| `/poke/agent/tasks/<ID>.json` | Agent 的任务去重与结果补报记录 |
| 角色目录下的 `state.lock` | 防止多个进程同时使用同一角色数据目录 |

私有 JSON 文件为 `0600`，更新采用同目录临时文件、文件同步、原子重命名与目录同步。损坏 JSON 会阻止启动，应从备份恢复；不要删除身份文件来绕过错误。

备份前停止对应实例，再备份整个角色数据目录以及 Controller 的配置和凭据文件。任务文件目前保留在磁盘，列表只展示最近 200 项；如需归档，应在停止实例后归档已确认终态的任务，并同时考虑 Controller 与 Agent 两侧记录。不要删除未补报或结果待确认的任务记录。

开发目录回退到 `server/data`；这些运行数据已加入忽略规则，不得提交或放入镜像构建上下文。

## 检查部署

- Controller 的 `/health` 返回 `OK`，HTTPS 入口支持 WebSocket 升级。
- Agent 注册后在页面显示在线，普通重启后可自动恢复连接。
- 节点可以查询 Docker 资源，私有镜像操作仅使用已授权仓库。
- 终端可双向收发，撤销节点后连接断开。
- 备份和恢复时同时保留 Controller 配置、节点状态及对应 Agent 身份。
