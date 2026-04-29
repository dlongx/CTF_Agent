# AI驱动的CTF自动化解题平台

这是一个OpenCode驱动的CTF自动化解题平台。当前项目已经收敛为单一Go后端、Gin+HTML前端和Docker隔离执行层：用户提交题目和附件，Go后端持久化任务、调度Docker容器、按需读取日志文件，容器内由OpenCodeWeb执行Agent解题。

## 当前能力

- Go后端:兼容REST API和WebSocket日志流，支持任务队列、并发Worker、任务恢复和文件持久化。
- Gin+HTML前端:任务提交、题目卡片列表、单题大屏、按需读取logs.txt和实时跟随。
- Docker调度:按题型选择镜像，限制CPU/内存/进程数，成功出Flag后销毁容器，未解出时保留容器供继续解题。
- OpenCode执行层:每个任务容器内启动OpenCodeWeb，根据题型生成Prompt并输出解题日志，任务详情页可打开当前OpenCode会话。

## 目录结构

```text
cmd/go-server/             # Go后端入口
internal/app/              # API、任务队列、Docker调度、WebSocket、存储
web/templates/             # Gin HTML模板
web/static/                # 内置前端CSS和JS
docker/agent-base/         # 通用CTF工具基础镜像
docker/opencode-agent/     # OpenCode执行镜像
docker/web-agent/          # Web题轻量专用镜像
docker/pwn-agent/          # Pwn题轻量专用镜像
docker/crypto-agent/       # Crypto题轻量专用镜像
docker/reverse-agent/      # Reverse题轻量专用镜像
docker/forensics-agent/    # Forensics题轻量专用镜像
docker/misc-agent/         # Misc题轻量专用镜像
runtime/opencode/          # 容器内OpenCode运行时桥接层
runtime/opencode/skills/   # 内置CTF题型Skill和参考资料
docs/                      # 架构和执行层文档
data/challenges/           # 本地任务数据，运行时生成
```

## 环境准备

需要本机可用：

```text
Go
Docker Desktop或Linux Docker Engine
```

复制模型配置：

```bat
copy opencode.env.example opencode.env
notepad opencode.env
```

`opencode.env`被`.gitignore`忽略，不要提交真实Key。

## GitHub上传前检查

仓库只应该提交源码、Dockerfile、模板、内置Skill和文档。以下内容只保留在本地：

```text
opencode.env              # 真实模型网关地址和Key
data/                     # 任务附件、日志、WP、Flag等运行数据
go-server.exe             # Windows本地构建产物
docker-build-*.log        # 镜像构建日志
__pycache__/              # Python缓存
```

首次推送前建议执行：

```bat
git status --ignored
```

确认`opencode.env`和`data/challenges/`没有出现在待提交文件里。

## 构建镜像

先构建基础镜像：

```bat
docker build --pull=false -t ctf-agent-base:latest docker/agent-base
```

再构建OpenCode镜像：

```bat
docker build -t ctf-agent-opencode:latest docker/opencode-agent
```

构建所有轻量题型专用镜像：

```bat
docker-build-light.bat
```

也可以按需单独构建：

```bat
docker build -t ctf-agent-web:latest docker/web-agent
docker build -t ctf-agent-pwn:latest docker/pwn-agent
docker build -t ctf-agent-crypto:latest docker/crypto-agent
docker build -t ctf-agent-reverse:latest docker/reverse-agent
docker build -t ctf-agent-forensics:latest docker/forensics-agent
docker build -t ctf-agent-misc:latest docker/misc-agent
```

如需切换npm源：

```bat
docker build -t ctf-agent-opencode:latest docker/opencode-agent ^
  --build-arg NPM_REGISTRY=https://registry.npmjs.org
```

## 启动开发环境

根目录运行：

```bat
start-dev.bat --restart
```

脚本会启动：

```text
Web UI和API: http://127.0.0.1:8000
```

`--restart`会清理占用`8000`的旧开发进程。

只启动服务：

```bat
go run ./cmd/go-server
```

## API

提交任务：

```bash
curl -X POST http://127.0.0.1:8000/api/tasks \
  -F "name=demo" \
  -F "type=misc" \
  -F "description=scan attachment" \
  -F "target_ip=127.0.0.1" \
  -F "attachments=@./challenge.txt"
```

查询任务：

```bash
curl http://127.0.0.1:8000/api/tasks
curl http://127.0.0.1:8000/api/tasks/{task_id}
curl http://127.0.0.1:8000/api/tasks/{task_id}/logs
```

实时日志：

```text
ws://127.0.0.1:8000/ws/tasks/{task_id}/logs
```

页面入口：

```text
http://127.0.0.1:8000/             # 题目卡片列表和提交表单
http://127.0.0.1:8000/tasks/{id}   # 单题大屏
```

## 任务数据

任务会持久化到：

```text
data/challenges/{task_id}/
  meta.json       # 任务名称、题型、状态、Flag、退出码等元数据
  logs.txt        # 容器stdout/stderr实时日志
  attachments/    # 用户上传的题目附件
```

Go后端启动时会扫描`data/challenges/*/meta.json`，状态为`queued`或`running`的任务会重新入队。

## 配置项

```text
CTF_AGENT_GO_ADDR=127.0.0.1:8000
CTF_AGENT_DATA_DIR=data
CTF_AGENT_DOCKER_IMAGE=ctf-agent-opencode:latest
CTF_AGENT_IMAGE_WEB=ctf-agent-web:latest
CTF_AGENT_IMAGE_PWN=ctf-agent-pwn:latest
CTF_AGENT_IMAGE_CRYPTO=ctf-agent-crypto:latest
CTF_AGENT_IMAGE_REVERSE=ctf-agent-reverse:latest
CTF_AGENT_IMAGE_FORENSICS=ctf-agent-forensics:latest
CTF_AGENT_IMAGE_MISC=ctf-agent-misc:latest
CTF_AGENT_CATEGORY_IMAGES=web=ctf-agent-web:latest,pwn=ctf-agent-pwn:latest
CTF_AGENT_MEM_LIMIT=512m
CTF_AGENT_CPUS=1.0
CTF_AGENT_MAX_CONTAINERS=4
CTF_AGENT_PIDS_LIMIT=1024
CTF_AGENT_DISABLE_NETWORK=false
CTF_AGENT_AGENT_SCRIPT=runtime/opencode/bridge.py
CTF_AGENT_SKILLS_DIR=runtime/opencode/skills
OPENCODE_PROVIDER_FORMAT=openai-compatible
OPENCODE_OPENAI_PROVIDER_ID=ctf
OPENCODE_OPENAI_PROVIDER_NAME=CTF Model Gateway
OPENCODE_OPENAI_PROVIDER_NPM=@ai-sdk/openai-compatible
OPENCODE_OPENAI_BASE_URL=https://your-model-gateway.example/v1
OPENCODE_OPENAI_API_KEY=your-openai-compatible-key
OPENCODE_OPENAI_MODEL=gpt-5.2
OPENCODE_ANTHROPIC_PROVIDER_ID=anthropic
OPENCODE_ANTHROPIC_PROVIDER_NAME=Anthropic
OPENCODE_ANTHROPIC_PROVIDER_NPM=@ai-sdk/anthropic
OPENCODE_ANTHROPIC_BASE_URL=https://api.anthropic.com/v1
OPENCODE_ANTHROPIC_API_KEY=your-anthropic-key
OPENCODE_ANTHROPIC_MODEL=claude-sonnet-4-5
OPENCODE_SERVER_PASSWORD=optional-password
CTF_AGENT_OPENCODE_WEB_ENABLED=true
CTF_AGENT_OPENCODE_BIND_IP=127.0.0.1
CTF_AGENT_OPENCODE_PUBLIC_BASE_URL=http://127.0.0.1
```

Go调度层和OpenCode桥接层不再设置解题总时长超时。任务会一直运行到解出Flag、容器内Agent退出，或用户在Docker管理页手动销毁容器。

`ctf-agent-opencode:latest`同时安装`@ai-sdk/openai-compatible`和`@ai-sdk/anthropic`。前端“模型接口格式”只切换当前使用哪一组配置，不接收、不展示API Key；Key仍来自`opencode.env`。切换后只影响新启动或继续运行时新执行的容器，不会改变已经启动的容器。

镜像选择规则：

```text
1.优先读取CTF_AGENT_IMAGE_{TYPE}，例如CTF_AGENT_IMAGE_PWN
2.也支持CTF_AGENT_CATEGORY_IMAGES=web=xxx,pwn=yyy
3.没有题型专用镜像时，回退到CTF_AGENT_DOCKER_IMAGE
```

## OpenCode执行模型

每个任务都会创建一个全新的Docker容器。容器内运行`runtime/opencode/bridge.py`：

```text
1.读取题目环境变量和/attachments附件目录
2.生成/workspace/opencode.json
3.启动opencode web --hostname 0.0.0.0 --port 4096
4.创建OpenCode会话
5.根据题型生成CTF解题Prompt
6.把OpenCode日志和结果输出到stdout
```

后端只捕获容器stdout/stderr，不直接调用模型供应商接口。模型协议、Key和权限策略交给OpenCode配置处理。

Flag提取规则：

```text
1.优先读取OpenCode最终可读输出的最后一个非空行
2.如果没有final readable OpenCode output块，则本次任务不判定Flag
```

因此Prompt会强制要求OpenCode把最终Flag放在最后一行，且最后一行只包含Flag本身。

OpenCodeWeb访问规则：

```text
1.后端为每个任务容器动态映射4096端口
2.任务运行中或未解出但保留容器时，详情页显示直达当前session的OpenCode会话入口
3.本地开发默认只绑定127.0.0.1
4.服务器部署时按实际域名或IP设置CTF_AGENT_OPENCODE_PUBLIC_BASE_URL
5.共享服务器建议设置OPENCODE_SERVER_PASSWORD
```

## 内置Skills

内置CTF Skill位于：

```text
runtime/opencode/skills/
  crypto.md
  web.md
  pwn.md
  reverse.md
  forensics.md
  misc.md
  reference/
```

Go后端会把`runtime/opencode/skills`只读挂载到容器内`/skills`，并传入：

```text
CTF_AGENT_SKILLS_DIR=/skills
CTF_AGENT_SKILL_IDS={题型}
```

`runtime/opencode/bridge.py`会按题型读取对应Skill并拼入OpenCode Prompt。单个Skill默认最多注入12000字符，避免Prompt过大。

## 安全边界

```text
每个任务一个全新容器
附件只读挂载到/attachments
容器CPU、内存和进程数受限
成功出Flag后自动删除容器
未解出任务容器会保留，用户可继续提示、打开OpenCode会话或在Docker管理页手动销毁
OpenCodeWeb默认只映射到宿主机127.0.0.1
```

## 自动检查

```bat
go test ./...
go build ./cmd/go-server
python -m py_compile runtime/opencode/bridge.py
```
