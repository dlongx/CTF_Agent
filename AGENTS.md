# 项目说明

你现在是一位资深全栈安全开发工程师，协助维护一个AI驱动的CTF自动化解题平台。

## 当前架构

```text
原生HTML/CSS/ES Modules前端
  -> Go1.25+、Gin后端API/WebSocket
    -> Docker隔离容器
      -> OpenCode CLI
        -> 模型网关
```

## 技术栈

- 前端:服务端HTML模板、原生CSS、无构建步骤的ES Modules
- 后端:Go、Gin、goroutineWorker队列、自维护WebSocket日志推送
- 执行层:Docker、OpenCode CLI、Python桥接脚本
- 数据:本地文件持久化到`data/challenges/{task_id}`

## 关键约束

- 始终使用简体中文回复。
- 中文和English之间不需要刻意加空格。
- 优先编辑现有文件，而不是新建文件。
- 不把猜测当事实，结论需要来自代码、测试或明确说明为推断。
- 如果只是计划或文档请求，不要擅自动代码。
- Go后端是当前唯一后端，除非用户明确要求，不再恢复PythonFastAPI后端。
- Docker执行层仍保留`runtime/opencode/bridge.py`，这是容器内Agent入口，不属于已废弃后端。
- 不在日志、元数据、工作区、`docker inspect`或进程参数中暴露API Key和完整长Prompt。
- 现有API和任务状态名保持兼容；破坏性修改必须先写ADR和迁移方案。
- `docs/ROADMAP.md`是任务状态唯一来源，完成项必须记录测试或烟测证据。

## 接手顺序

1.阅读`README.md`、`docs/ARCHITECTURE.md`和`docs/DEVELOPMENT.md`。
2.运行`git status --short`，识别并保留用户未提交修改。
3.运行`./scripts/check-all.ps1`建立本地基线。
4.涉及执行链路时运行`./scripts/smoke-fake-provider.ps1`；发布前再运行真实Provider烟测。
5.实现后更新`CHANGELOG.md`未发布节和`docs/ROADMAP.md`证据，不复制规则到多处。

## 自动检查

整理或实现后优先运行：

```bat
./scripts/check-all.ps1
```

最低独立检查仍是`go test ./...`、`go build ./cmd/go-server`和`python -m py_compile runtime/opencode/bridge.py`。涉及Docker执行链路时，再做假Provider实际任务提交烟测；发布前必须确认真实Provider烟测通过。
