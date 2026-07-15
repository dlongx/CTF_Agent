# 运维手册

## 启动与关闭

```powershell
go build -o ./bin/ctf-agent.exe ./cmd/go-server
./bin/ctf-agent.exe
```

使用Ctrl+C或发送正常终止信号。服务会取消所有Agent运行并等待Worker，最多10秒后清理仍运行的受管容器。不要使用强制结束作为常规关闭方式。

非回环监听必须设置强随机`CTF_AGENT_ACCESS_TOKEN`，并在反向代理层提供TLS。当前项目不支持直接公网多租户部署。

## 健康检查

- `GET /health`:仅确认HTTP进程存活，始终返回简短状态。
- `GET /ready`:检查数据目录写入、活动Provider配置、Docker服务及配置镜像。依赖异常返回HTTP503。
- `POST /api/settings/provider/test`:由操作者显式触发，访问活动配置的`/models`。返回脱敏错误码，不返回URL、Key、响应体或任务内容。

常见Provider错误码:`unauthorized`、`rate_limited`、`upstream_http_522`、`timeout`、`network_error`、`invalid_response`。HTTP522通常表示外部网关故障，应检查Provider状态，不应自动换模型。

## 容器与磁盘

Docker管理页展示运行数、跟踪数、容器状态、可写层磁盘占用和上次清理结果。失败容器默认保留24小时，每小时清理；服务启动也会回收过期孤儿容器。

临时立即清理单个任务应使用任务停止或容器关闭API，不要按模糊名称批量删除Docker资源。调整保留时间后需要重启服务。

任务日志每份默认10MiB，保留3份归档。容量规划上单任务日志最坏约40MiB，附件和WP另计。每月检查`data`和Docker磁盘占用。

## 备份与恢复

先正常停止服务，再执行:

```powershell
./scripts/backup-data.ps1 -OutputDir ./backups
./scripts/restore-data.ps1 -Archive ./backups/ctf-agent-data-YYYYMMDD-HHMMSS.zip
```

备份包含SHA-256清单。恢复先解压到同盘临时目录、校验所有清单文件，再替换数据目录；旧目录保留为`.previous-*`。恢复后运行`GET /ready`、检查任务列表，并提交假Provider烟测。每季度实际演练一次。

## 故障处理

1.保存`git rev-parse HEAD`、脱敏启动日志、`/ready`结果和相关`task_id`。
2.检查任务原始轮转日志和Docker管理页，不复制Key或完整敏感附件。
3.若Provider异常，运行显式连通性测试并记录错误码；不要反复提交付费请求。
4.若元数据损坏，先备份；服务会尝试`.bak`恢复，失败目录不会加载。
5.若Docker异常，先确认Engine和镜像，再检查容器限制、挂载和磁盘空间。

## 发布

采用SemVer标签。发布前要求CI、全镜像构建、假Provider烟测、真实Provider烟测、备份恢复演练和密钥泄露检查全部通过，并把变更从`Unreleased`归档到版本节。
