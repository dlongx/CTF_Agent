# 开发指南

## 环境

- Go1.25或1.26
- DockerDesktop或兼容DockerEngine
- Python3.12+
- Node.js24用于JavaScript语法检查
- PowerShell7用于统一脚本

```powershell
git clone <repository-url>
cd CTF_Agent
Copy-Item opencode.env.example opencode.env
./scripts/check-all.ps1
```

未配置真实Provider时，单元测试和假Provider烟测仍可运行。不要把测试改成依赖公网、真实Key或模型额度。

## 配置来源

服务先读取进程环境，再读取根目录`opencode.env`补充空缺项。`opencode.env`被Git忽略。Provider支持`openai-compatible`和`anthropic`两种格式，每种包含`PROVIDER_ID`、`PROVIDER_NAME`、`PROVIDER_NPM`、`BASE_URL`、`API_KEY`和`MODEL`。

运行限制变量:

```text
CTF_AGENT_TASK_TIMEOUT=0s
CTF_AGENT_OPENCODE_RUN_TIMEOUT=0s
CTF_AGENT_OPENCODE_IDLE_TIMEOUT=0s
CTF_AGENT_AUTO_CONTINUE_ROUNDS=6
CTF_AGENT_CONTAINER_RETENTION=24h
CTF_AGENT_LOG_MAX_BYTES=10485760
CTF_AGENT_MAX_CONTAINERS=1
CTF_AGENT_MEM_LIMIT=auto
CTF_AGENT_CPUS=auto
```

所有时长使用Go duration格式。`AUTO_CONTINUE_ROUNDS=0`明确关闭自动续跑。完整默认值以`internal/app/config.go`为唯一代码来源。
任务、单轮和空闲超时均可设为`0s`关闭。资源值为`auto`时，任务独占除1个CPU和至少1GiB内存之外的Docker可用资源。
`CTF_AGENT_CONTAINER_RETENTION=0s`关闭保留容器和孤儿容器的定时清理，适合共享Docker守护进程的隔离烟测。

## 本地开发

```powershell
go run ./cmd/go-server
```

前端不需要构建。修改`web/templates`、`web/static`后重启Go服务，并在桌面与移动视口检查任务列表、任务详情、断线重连和设置面板。

## 测试层次

```powershell
./scripts/check-all.ps1
./scripts/check-all.ps1 -Vulnerability
./scripts/smoke-fake-provider.ps1
```

`-Race`要求启用CGO和可用C编译器，默认由Linux CI执行；本机不具备该条件时，不要用跳过的本机结果替代CI证据。

`smoke-fake-provider.ps1`构建本地假Provider和Go服务，提交misc任务，实际运行Docker/OpenCode并验证Flag与WP。发布候选还必须运行`smoke-opencode.bat`，真实Provider失败时保留错误码并停止，不得切换Provider。

测试用例至少覆盖正常响应、401、429、522、超时、无效JSON，以及OpenAI兼容和Anthropic认证头。生命周期变更要覆盖轮数上限、并发继续、队列满、取消、关闭和重启恢复。

## 依赖与镜像

- Go依赖由`go.mod`和`go.sum`固定，升级后运行`go mod tidy -diff`和`govulncheck`。
- Python直接依赖记录在各镜像`requirements.lock`；OpenCode和Provider包版本在Dockerfile中显式固定。
- 基础Python镜像固定到Digest。更新Digest时记录上游标签、架构和完整重建结果。
- 一次PR只升级一个生态。月度全镜像工作流用于发现上游漂移。

## 完成定义

代码已格式化、确定性检查通过、相关烟测通过、没有泄露密钥、文档与行为一致、`CHANGELOG.md`和`docs/ROADMAP.md`已更新。无法执行的真实外部检查必须明确记录为发布阻塞，而不是宣称通过。
