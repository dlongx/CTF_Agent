# 架构

## 系统边界

```text
浏览器
  | HTTP、SSE、WebSocket
Go/Gin单体服务
  | 文件系统                 | Docker CLI
data/challenges/{task_id}    每任务一个题型容器
                               | docker exec环境变量
                             bridge.py -> OpenCode CLI -> 显式Provider
```

服务只负责认证、API、队列、持久化、Docker生命周期和日志转发，不直接提交模型推理请求。`POST /api/settings/provider/test`是独立诊断，仅请求Provider的`/models`。

## 组件职责

|组件|职责|不负责|
|---|---|---|
|`cmd/go-server`|配置加载、结构化日志、HTTP生命周期|业务规则|
|`internal/app/router.go`|HTTP/SSE/WebSocket入口、验证和响应码|Docker实现细节|
|`internal/app/service.go`|Worker、续跑、取消、恢复、容器保留|文件格式细节|
|`internal/app/store.go`|任务状态、原子元数据、日志轮转|网络和Docker|
|`internal/app/docker.go`|容器创建、执行、读取和销毁|任务状态决策|
|`runtime/opencode/bridge.py`|Prompt、OpenCode事件解析、超时、结果协议|后端API|
|`web/static`|原生浏览器交互|安全决策|

## 任务数据流

1.API验证题型、字段、Provider和附件，创建`queued`任务并写入`meta.json`。
2.有界队列把任务交给最多`MaxContainers`个Worker。
3.Worker标记`running`，创建容器，执行桥接脚本并把stdout/stderr追加到轮转日志。
4.桥接层按题型装载Skill，使用临时Prompt文件启动OpenCode，解析JSON事件和sessionID。
5.检测到严格两行Flag协议后保存WP、标记`solved`并销毁容器。
6.没有Flag时在同一session最多自动续跑配置轮数；耗尽后标记`completed`并保留容器。
7.人工继续也进入同一Worker队列。同一任务仅允许一个待处理或运行的继续请求。

Flag与WP的严格结果协议见[Flag提取逻辑](flag-extraction.md)。

## 故障与恢复

- 写入`meta.json`前先保留有效的`.bak`，通过同目录临时文件、fsync和替换提交。
- 启动时迁移旧schema并从有效`.bak`恢复损坏元数据。
- 启动时把没有保留容器的`queued/running`任务重新入队；有容器的中断任务保留供诊断。
- 服务关闭取消全部运行，等待Worker，10秒后强制清理仍在运行的受管容器。
- 每小时清理超过保留期的失败容器和孤儿容器，并协调本地元数据。

## 安全模型

这是单机、受信任操作者模型。每任务一个容器，附件只读挂载，容器设置CPU、内存、PIDs限制并默认以UID1000运行。网络默认开启，因为多数CTF题需要访问目标；需要离线处理时设置`CTF_AGENT_DISABLE_NETWORK=true`。

API Key仅在`docker exec`环境中存在，不属于容器创建配置。OpenCode配置使用`OPENCODE_CONFIG_CONTENT`和环境变量引用。服务日志只输出脱敏配置和任务/容器标识。

## 扩展阈值

- 任务超过10,000、需要跨任务复杂查询或多进程写入时，再以ADR评估SQLite。
- 原生前端超过3,000行或出现多个复杂交互页面时，再评估Vue。
- 在需要公网多租户前，不引入RBAC或微服务；当前安全模型不支持该用法。

关键决策见[ADR](adr/README.md)。
