# OpenCode运行时权限设计

## 结论

可以让容器里的OpenCode进入“最大权限”模式，避免解题时频繁询问命令、读写、子代理、外部目录等操作是否允许。

推荐的理解方式是：

```text
OpenCode层面:尽量自动批准，不打断Agent
Docker层面:继续强隔离，限制Agent能伤害的范围
```

也就是说，最大权限只应该发生在单题Docker容器内部，不能等价为宿主机最大权限。容器仍然必须保留CPU、内存、进程数、capability、no-new-privileges、挂载目录只读等限制。

## 当前执行链路

```text
Gin+HTML前端
  -> Go API/SSE/WebSocket
    -> Docker任务容器
      -> runtime/opencode/bridge.py
        -> opencode run --format json
          -> 模型网关
```

Go后端负责控制面：

```text
任务提交
附件保存
队列和并发Worker
按题型选择Docker镜像
Docker资源限制
日志文件持久化
WebSocket日志推送
SSE任务状态事件
OpenCode终端session持久化
中文标记下一行Flag提取
```

OpenCode负责容器内执行面：

```text
模型调用
会话上下文
读取附件
写入/workspace临时脚本
执行shell命令
调用内置工具和子代理
生成解题输出
```

## 最大权限如何实现

OpenCode官方权限模型支持三种动作：

```text
allow:无需审批直接运行
ask:提示审批
deny:阻止操作
```

如果想让所有OpenCode权限默认自动通过，可以在容器内`/workspace/opencode.json`写入：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "permission": "allow"
}
```

这种写法比逐个配置`bash`、`edit`、`read`、`external_directory`更直接，也能覆盖默认会询问的`external_directory`和`doom_loop`场景。

当前`runtime/opencode/bridge.py`会为每道题生成`/workspace/opencode.json`，并已经使用最大权限模式：

```python
opencode_config = {
    "$schema": "https://opencode.ai/config.json",
    "model": f"{config.provider_id}/{config.model}",
    "small_model": f"{config.provider_id}/{config.model}",
    "permission": "allow",
    "provider": {
        config.provider_id: provider_config,
    },
}
```

## 为什么不能只靠OpenCode权限控制安全

CTF附件和题目环境天然不可信。让OpenCode自动批准命令会提升解题效率，但也意味着模型可以连续执行更激进的命令，例如：

```text
批量解压和递归扫描
运行未知二进制
执行爆破脚本
启动网络探测
写入临时利用脚本
反复调用同一工具
读取工作目录外路径
```

这些行为对CTF是有价值的，但不能让它们越过容器边界。因此安全设计应该把“审批边界”从OpenCode迁移到Docker隔离层。

## 必须保留的Docker边界

最大权限模式下，以下边界不要放松：

```text
每题一个全新容器
任务结束后销毁干净环境
不挂载Docker socket
不使用--privileged
不挂载宿主机敏感目录
附件目录/attachments只读挂载
Skill目录/skills只读挂载
Agent脚本只读挂载
工作目录限定在/workspace
限制内存CTF_AGENT_MEM_LIMIT
限制CPU CTF_AGENT_CPUS
限制进程数CTF_AGENT_PIDS_LIMIT
cap-drop=ALL
security-opt=no-new-privileges:true
按需决定是否允许网络
```

当前项目的Docker启动策略已经覆盖大部分边界：

```text
--memory
--cpus
--pids-limit
--cap-drop ALL
--security-opt no-new-privileges:true
--tmpfs /tmp:rw,noexec,nosuid,size=64m
-w /workspace
/attachments:ro
/skills:ro
/opt/ctf_agent/agent.py:ro
```

## 推荐权限策略

### 开发和本地CTF

本地开发、单人使用、任务容器不暴露到公网时，推荐：

```text
OpenCode permission:allow
Docker网络:按题目需要开启
OpenCode执行:opencode run --format json
容器失败后保留，支持在终端下方继续发送消息
成功出Flag后自动关闭容器
```

这是效率最高的模式，适合当前平台的主要目标。

### 共享服务器

多人使用或部署到服务器时，推荐：

```text
OpenCode permission:allow
前端和API放在受控网络或反向代理认证后
限制每用户并发数
限制容器网络出口
记录容器stdout/stderr和任务元数据
失败容器设置保留时间上限
定期清理孤儿容器和旧任务
```

### 高风险比赛环境

如果题目附件来源不可信且可能包含恶意样本，推荐：

```text
OpenCode permission:allow或按题型降级
Docker网络默认关闭
只给Web题或远程服务题开启网络
禁止宿主机目录写挂载
保留更短的任务超时
使用更小权限的题型专用镜像
```

## 不推荐的做法

```text
把宿主机Docker socket挂进容器
使用--privileged运行题目容器
把项目根目录以可写方式挂进容器
把opencode.env或真实API Key挂进容器文件系统
把真实模型Key写入Docker镜像
成功解题后永久保留容器
为了方便调试取消CPU/内存/进程限制
```

这些做法会让“容器内最大权限”变成“宿主机风险”。

## 与Flag提取的关系

权限策略不负责判断Flag。当前平台只使用一条主规则：

```text
精确匹配一行“这道题目已经解出”，捕获下一条非空、非平台日志行作为Flag
```

`runtime/opencode/bridge.py`会要求OpenCode解出后按以下两行收尾：

```text
这道题目已经解出
<exact flag>
```

Go后端会优先按这个中文标记捕获Flag，并保留旧final readable output最后一行和`flag{}`正则作为历史兼容兜底。

## 终端事件流

OpenCode通过`opencode run --format json`输出JSON事件流。桥接脚本会递归提取：

```text
OpenCodesessionID
assistant可读文本
工具输出
最终中文Flag标记块
```

Go调度层不设置解题总时长超时。失败任务不会立即删除容器，用户可以在详情页终端下方补充消息，继续同一个OpenCodesession。

## 当前验证步骤

```text
1.保留Docker所有资源和安全限制
2.本地烟测一题，确认不再出现permission asking日志
3.测试需要外部目录/重复命令/脚本写入的题目
4.确认任务日志里没有旧Web链路、4096端口映射或export session旧链路输出
```

## 相关文件

```text
runtime/opencode/bridge.py
internal/app/docker.go
internal/app/service.go
internal/app/router.go
docker/agent-base/Dockerfile
docker/opencode-agent/Dockerfile
docker/*-agent/Dockerfile
opencode.env.example
```

## 参考

```text
OpenCode权限文档:
https://opencode.ai/docs/zh-cn/permissions/
```
