# Controller / Agent 部署

Agent 主动连接 Controller，管理状态统一在 Controller 以 JSON 持久化，Agent 不保存身份或任务文件。

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
| `POKE_NODE_TOKEN` | 长期节点凭据，必须保留至轮换或撤销 |
| `POKE_CA_FILE` | 可选，Agent 信任的私有 CA 文件 |
| `POKE_ALLOW_INSECURE_HTTP=true` | 仅隔离测试环境允许明文 HTTP；默认关闭 |

## 首次接入

管理员默认用户名为 `admin`。Web 首次初始化成功后，日志会打印一次用户名和随机密码；重启不会重复打印或重置账号。登录需要同时填写用户名和密码，可在“基本设置”修改用户名。用户名为 1–64 个字符，以中文、字母或数字开头，支持中文、字母、数字、下划线、点、`@` 和连字符；已有配置缺少用户名时会补为 `admin` 并保留原密码。

仓库提供两份独立 Compose 文件：主节点使用根目录 `compose.yaml`，各从节点使用 `compose.agent.yaml`。两端分别复制 `.env.example` 为本机 `.env`，默认使用 `ghcr.io/a1zaza/poke:latest`；可将 `POKE_IMAGE` 设置为固定版本，例如 `ghcr.io/a1zaza/poke:1.0.0`。Controller 域名需替换为实际地址。

主节点配置镜像、端口及 HTTPS 入口后，由部署者执行：

```bash
docker compose -f compose.yaml up -d
```

从节点先确认 Docker socket 可用、业务绝对路径已准备好，在本机 `.env` 中填写 Controller 地址、节点 ID 和 `POKE_NODE_TOKEN`，再执行：

```bash
docker compose -f compose.agent.yaml up -d
```

两个文件不合并运行。Controller 默认仅管理远程节点，不挂载本机 Docker socket；Agent 无端口映射。Controller 使用独立命名数据卷，Agent 默认仅挂载 Docker socket，不映射 `/poke`；Compose 运行副本在容器内按需生成，节点、任务和源配置 JSON 保存在 Controller。`POKE_NODE_TOKEN` 必须长期保留，不再使用临时注册或本地身份文件。

`.env` 已加入 Git 忽略规则，不要提交真实注册凭据。业务目录在 Agent 所连接 Docker 的宿主机上，由 Compose 通过绝对路径挂载到业务容器，不需要经过 Agent 容器。启动前应确认 Docker socket 和业务目录存在。

1. 以 Controller 模式部署当前 Poke 镜像，持久化 `/poke`。容器监听 80，对外提供受信任的 HTTPS 地址。
2. HTTPS 入口需支持 `/agent/v1/connect`、`/agent/v1/stream` 和 `/ws/*` 的 WebSocket 升级，空闲连接超时应大于 60 秒。HTTPS 管理入口继续传递 `X-Forwarded-Proto: https` 或使用 `COOKIE_SECURE=true`。
3. 登录 Controller，进入“节点与任务”。需要私有仓库时，先在“镜像仓库”保存认证，再回到“节点与任务”输入名称、选择授权仓库并生成长期节点 Token。
4. 在 Agent 主机的 `.env` 中填写 `POKE_NODE_TOKEN=<长期节点Token>`。不要把实际凭据写入镜像、仓库、URL 或日志。
5. 使用同一 Poke 镜像设置 `POKE_MODE=agent`、`POKE_CONTROLLER_URL`、`POKE_NODE_ID` 和 `POKE_NODE_TOKEN`。仅挂载 Docker socket；业务 bind mount 使用宿主机绝对路径，例如 `/srv/app/data:/data`。无需映射 Agent 端口。建议启用容器运行时 `init` 和失败重启策略。
6. Agent 通过 ID/Token 认证并主动连接后，页面显示“在线”。选择该节点即可管理 Docker 资源。
7. 长期保留 `POKE_NODE_ID`、`POKE_NODE_TOKEN` 和 Controller 地址。相同 ID 与 Token 的重启视为同一节点；同一节点只允许一个活动实例，不要将同一份身份同时部署到多台主机。

Controller 持久化管理数据，Agent 无需管理状态数据卷。更换 Controller 或节点身份时，应先停止原实例，确认原任务状态，再更新对应的地址、ID 和 Token；不能通过复制 Token 实现并行多副本。Agent 地址无需配置到 Controller，也不要求 Controller 能直接连接 Agent。默认模板不共享项目目录，不应使用 `./data:/data` 等相对 bind mount。若自行启用相对路径，需显式增加 `/poke/projects:/poke/projects`，保证 Agent 与 Docker 宿主机的项目路径一致。

Agent 与 Docker 的通信仍取决于 Docker socket 或 `DOCKER_HOST`；节点控制通道在线表示管理连接正常，Docker 操作失败会作为明确的接口或任务错误返回。

## 可选自定义配置

Compose 模板仅保留必要配置；按需在部署副本中增加以下内容，不需要全部启用。

| 需求 | 调整方式 |
| --- | --- |
| 修改主节点端口或绑定地址 | 修改 `ports`，例如 `127.0.0.1:8080:80`；容器端口保持 80 |
| 强制 HTTPS Cookie | Controller 的 `environment` 增加 `COOKIE_SECURE: "true"`；HTTP 初始化访问时不要启用 |
| 私有 CA | 单独只读挂载 `./ca.pem:/run/poke-ca.pem:ro`，Agent 的 `environment` 增加 `POKE_CA_FILE: /run/poke-ca.pem` |
| 自定义时区 | 服务的 `environment` 增加 `TZ: Asia/Shanghai` |
| Rootless Docker | 将 Agent 的 socket 挂载源改为实际路径，容器目标仍为 `/var/run/docker.sock` |
| 日志轮转 | 服务下增加 `logging`，使用 `json-file`，设置 `max-size: "10m"`、`max-file: "3"` |
| 停止等待时间 | 服务下增加 `stop_grace_period`，建议 Controller 为 `15s`、Agent 为 `30s` |
| Controller 健康检查 | 服务下增加 `healthcheck`，在容器内请求 `http://127.0.0.1:80/health` |
| Controller 管理本机 Docker | 增加 `POKE_LOCAL_ENABLED: "true"`，并挂载 Docker socket 与 `/poke/projects` |

请保留 Controller 数据卷和 Agent 的长期身份环境变量。Agent 容器内 `/poke/projects` 只生成 Compose 运行副本，容器重建后可从 Controller 再次下发；业务文件保存在 Docker 宿主机的绝对路径。认证临时文件位于 `/tmp`，模板已配置 tmpfs。

## 身份和权限管理

容器页的更新检测错误标签可点击查看详情，包括本次操作、仓库、节点是否提供认证及 Docker 原始错误。“未配置认证”需检查仓库设置和节点授权；“认证失败”需检查凭据和镜像权限；“镜像未找到”需核对镜像/标签及权限；“连接失败”或“仓库异常”需检查网络、TLS 或代理。缺少凭据时的 404 不能证明仓库一定私有或镜像不存在，配置认证后应再次检查。公开镜像可匿名检测。

- 长期 Token 只验证对应节点，Controller 只保存摘要；遗失时在节点页面轮换。轮换后更新 Agent 环境变量并重建容器，原 Token 不能重连。
- 第二个不同进程使用相同身份时，返回 NODE_ALREADY_CONNECTED 或 NODE_RECONNECT_PENDING；请先停止重复实例。异常退出后通常需等待最长 60 秒租约到期。
- 未知旧任务会冻结节点修改，查询仍可用。停止原执行实例并核实资源后，可在“节点与任务”确认解除对应任务的阻塞。
- 节点连接凭据彼此独立；Controller 只保存摘要，Agent 从环境读取长期秘密，不生成身份文件。
- 页面可调整某节点的仓库授权。保存后断开该节点当前连接，Agent 使用原身份重新连接；后续操作按新授权取凭据。
- “轮换 Token”保留节点 ID 并生成新 Token。旧 Token 立即拒绝重连，旧租约与在途任务继续按防重复规则处理；停止原实例后更新环境变量并重建 Agent。
- “撤销节点”立即关闭该节点当前的控制、查询与终端连接，拒绝后续连接；已启动容器继续运行。已交给 Docker 的操作不能承诺回滚。

## 操作与任务

远程查询通过控制连接通知 Agent，再由 Agent 主动建立单独的数据连接返回 JSON。终端另用独立的双向数据连接；页面断开后不会重放终端输入。日志保持行数受限的查询方式。

远程修改先记录任务 ID，Agent 在内存排队、获得持久执行许可后执行。页面等待执行结果，也可在“节点与任务”中查看最近 200 项任务。Agent 在内存串行处理修改，执行前需取得 Controller 已持久保存的唯一许可；不确定任务禁止自动重放。

取消操作是尽力而为：尚未开始的任务可标记已取消；执行期间超时、取消或进程崩溃的任务标记“结果待确认”，需要检查实际 Docker 状态。浏览器关闭不会自动取消已经接收的修改任务。同一 Agent 进程断线后可以补报内存中的结果；进程重启会丢失这些结果，相关已许可任务进入待确认状态，不自动重放。

仓库秘密只随授权的相关操作临时发送，不写入任务记录；Compose 在执行前解析项目实际引用的镜像，将需要的仓库授权写入临时私有 `DOCKER_CONFIG`，结束后清理；Agent 模板将 `/tmp` 设为 tmpfs，容器重建清理遗留临时凭据，不使用 Agent 状态锁。镜像 SDK 与 Compose 使用相同的操作级凭据范围。

当前 standalone/Controller 本机管理使用进程内 API，远程 Agent 修改使用任务接口。系统不提供跨节点调度或主节点自动故障切换。

## JSON 文件布局与备份

| 文件 | 内容 |
| --- | --- |
| `/poke/config.json` | Controller/standalone 的普通设置、管理员用户名与密码哈希 |
| `/poke/secrets.json` | 私有仓库用户名和密码 |
| `/poke/controller/nodes.json` | 节点信息、仓库授权、长期 Token 摘要及持久租约 |
| `/poke/controller/tasks/<ID>.json` | Controller 的任务状态与结果 |
| `/poke/controller/projects/<节点>-<项目摘要>.json` | Controller 的项目源配置与版本历史 |
| Controller 目录下的 `state.lock` | 防止多个进程同时使用同一角色数据目录 |

私有 JSON 文件为 `0600`，更新采用同目录临时文件、文件同步、原子重命名与目录同步。损坏 JSON 会阻止启动，应从备份恢复；不要用空文件覆盖损坏状态。

备份前停止对应实例，再备份整个角色数据目录以及 Controller 的配置和凭据文件。任务文件目前保留在磁盘，列表只展示最近 200 项；如需归档，应在停止实例后归档已确认终态的任务，并保留相应项目版本及节点租约。不要删除未补报或结果待确认的任务记录。

开发目录回退到 `server/data`；这些运行数据已加入忽略规则，不得提交或放入镜像构建上下文。

## 检查部署

- Controller 的 `/health` 返回 `OK`，HTTPS 入口支持 WebSocket 升级。
- Agent 注册后在页面显示在线，普通重启后可自动恢复连接。
- 节点可以查询 Docker 资源，私有镜像操作仅使用已授权仓库。
- 终端可双向收发，撤销节点后连接断开。
- 备份和恢复时同时保留 Controller 配置、节点状态及Agent 的长期身份环境变量。

## 远程项目源配置

Controller 保存每个节点的项目源配置、当前版本及已部署版本；节点离线时可以查看和编辑源配置，保存不会部署；在线后通过“部署配置”应用选定的当前版本，运行状态不可用时会明确提示。外部项目在详情页通过“导入配置”提供完整 Compose 内容，导入仅保存配置，不立即部署。导入后须核对业务挂载均使用 Docker 宿主机绝对路径，避免依赖 Agent 容器内的工作目录。

远程项目首版不支持本地 build、env_file、include、extends、label_file、develop 和外部 configs/secrets 文件，也不支持宿主环境变量插值；请使用独立镜像和内联配置，容器内变量用 `$$` 转义。业务 bind mount 仍属于执行主机，移除远程项目不会删除业务目录。

本版 Controller/Agent 协议及节点状态文件不兼容旧方案。按全新环境部署成套版本，不复制旧 Agent 的 identity.json 或 tasks 目录；已有 Docker 业务数据不自动删除或迁移。

## Docker API 兼容

后端使用官方 `github.com/moby/moby/client v0.6.3` 与 `github.com/moby/moby/api v1.57.0`，默认自动协商 Docker Engine API，当前 SDK 支持 API 1.40–1.57。目标守护进程最低要求 API 1.44 时，不再受旧 v24 SDK 的 1.43 上限限制。

一般不要设置 `DOCKER_API_VERSION`。该变量会固定请求版本、覆盖自动协商；如果遗留值为 `1.43`，即使升级 Poke，仍可能被最低要求 1.44 的 Docker 拒绝。升级后应重新创建使用新镜像的 Agent（standalone 则更新 Web 容器），仅升级容器内 Docker CLI 不会更新 Go SDK。
