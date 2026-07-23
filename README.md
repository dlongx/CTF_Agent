# CTF_Agent

CTF_Agent是一个面向个人本机部署的AI驱动CTF自动化解题平台。用户提交题目与附件后，Go服务负责持久化和排队，Docker为每个任务创建隔离环境，容器内OpenCode通过Python桥接脚本调用用户配置的模型Provider。

项目选择继续维护现有Go单体，而不是重写。当前范围刻意保持克制:原生HTML/ES Modules前端、文件存储、单机Docker，不引入Vue、SQLite、微服务、RBAC或公网多租户。

## 功能

- REST API、WebSocket实时日志和无构建步骤的原生前端。
- goroutineWorker队列、并发上限、总任务/单轮/无输出超时及可靠关闭。
- web、pwn、crypto、reverse、forensics、misc题型镜像。
- OpenCode同一session自动续跑和人工继续，自动续跑最多6轮。
- 元数据原子写入、`.bak`恢复、版本迁移和任务日志轮转。
- 存活、就绪、Provider连通性和Docker容器诊断。
- 本地假Provider端到端烟测，不消耗真实模型额度。

## 快速开始

需要Go1.25或1.26、DockerDesktop、Python3.12+、Node.js。首次构建:

```powershell
docker build --pull -t ctf-agent-base:latest docker/agent-base
docker build -t ctf-agent-opencode:latest docker/opencode-agent
docker build -t ctf-agent-misc:latest docker/misc-agent
```

复制`opencode.env.example`为不提交到Git的`opencode.env`，填写一组Provider配置，然后启动:

```powershell
./start-dev.bat --restart
```

打开`http://127.0.0.1:8000`。只运行服务可使用`go run ./cmd/go-server`。

## 确定性检查

```powershell
./scripts/check-all.ps1
./scripts/smoke-fake-provider.ps1
```

`check-all.ps1`执行格式、依赖一致性、单测、vet、构建、Python、JavaScript及Markdown链接检查。LinuxCI额外执行`go test -race ./...`和`govulncheck ./...`。真实Provider烟测仅在发布前运行:

```powershell
./smoke-opencode.bat
```

假Provider烟测是确定性的；真实烟测依赖外部Provider，失败不会触发静默切换。

## 配置

常用服务配置:

|变量|默认值|说明|
|---|---:|---|
|`CTF_AGENT_GO_ADDR`|`127.0.0.1:8000`|监听地址|
|`CTF_AGENT_ACCESS_TOKEN`|空|非回环监听时必填|
|`CTF_AGENT_ALLOWED_ORIGINS`|空|明确允许的跨域来源|
|`CTF_AGENT_DATA_DIR`|`data`|本地数据根目录|
|`CTF_AGENT_MAX_CONTAINERS`|`1`|Worker和运行容器上限；默认单任务独占|
|`CTF_AGENT_TASK_TIMEOUT`|`0s`|任务总超时；`0s`表示不限时|
|`CTF_AGENT_OPENCODE_RUN_TIMEOUT`|`0s`|单次OpenCode运行超时；`0s`表示不限时|
|`CTF_AGENT_OPENCODE_IDLE_TIMEOUT`|`0s`|OpenCode无输出超时；`0s`表示不限时|
|`CTF_AGENT_AUTO_CONTINUE_ROUNDS`|`6`|自动续跑轮数，`0`表示不续跑|
|`CTF_AGENT_CONTAINER_RETENTION`|`24h`|未解出容器保留时间；`0s`表示关闭自动清理|
|`CTF_AGENT_LOG_MAX_BYTES`|`10485760`|单份任务日志上限，保留3份归档|
|`CTF_AGENT_MEM_LIMIT`|`auto`|自动使用Docker可用内存减去10%且至少保留1GiB|
|`CTF_AGENT_CPUS`|`auto`|自动使用Docker可用CPU并保留1核|
|`CTF_AGENT_PIDS_LIMIT`|`1024`|单容器进程数限制|
|`CTF_AGENT_DISABLE_NETWORK`|`false`|关闭任务容器网络|

镜像可用`CTF_AGENT_DOCKER_IMAGE`设置默认值，并用`CTF_AGENT_IMAGE_WEB`等题型变量覆盖。完整变量和Provider格式见[开发指南](docs/DEVELOPMENT.md)。

API Key只由服务环境传入一次执行进程。桥接层通过`OPENCODE_CONFIG_CONTENT`和`{env:OPENCODE_API_KEY}`引用Key，不写入任务工作区；长Prompt通过权限为`0600`的临时文件传给OpenCode，不进入进程参数。

## 数据与任务状态

```text
data/
  provider.json
  challenges/{task_id}/
    meta.json
    meta.json.bak
    logs.txt
    logs.txt.1 ... logs.txt.3
    attachments/
    *_wp.md
```

现有任务状态名保持兼容:`queued`、`running`、`solved`、`completed`、`failed`。到达自动续跑上限的任务进入`completed`并保留容器；同一任务并发继续返回HTTP409，队列满返回HTTP429。详细约定见[数据模型](docs/DATA_MODEL.md)和[API文档](docs/API.md)。

备份和恢复前应停止服务，避免跨文件时间点不一致:

```powershell
./scripts/backup-data.ps1
./scripts/restore-data.ps1 -Archive ./backups/ctf-agent-data-YYYYMMDD-HHMMSS.zip
```

恢复会校验每个文件的SHA-256，并把原数据目录保留为带时间戳的`.previous-*`目录。

## 项目结构

```text
cmd/go-server/             Go服务入口
cmd/fake-provider/         确定性测试Provider
internal/app/              配置、API、队列、存储、Docker和WebSocket
web/templates/             HTML模板
web/static/                CSS和原生ES Modules
docker/*-agent/            分层Docker镜像
runtime/opencode/bridge.py 容器内Agent入口
runtime/opencode/skills/   OpenCode原生Skills和引用资料
scripts/                   检查、烟测、备份和恢复
docs/                      架构、开发、运维、接口、ADR和路线图
```

## 维护入口

- 新维护者先读[AGENTS.md](AGENTS.md)和[开发指南](docs/DEVELOPMENT.md)。
- 系统边界与数据流见[架构文档](docs/ARCHITECTURE.md)。
- 部署、诊断、清理与恢复见[运维手册](docs/OPERATIONS.md)。
- 任务状态的唯一来源是[路线图](docs/ROADMAP.md)。
- 设计取舍记录在[ADR目录](docs/adr/README.md)。
- 安全问题按[安全策略](SECURITY.md)报告。

本项目使用[MIT许可证](LICENSE)。
