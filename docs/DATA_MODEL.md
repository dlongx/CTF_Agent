# 数据模型

## 目录

每个任务位于`data/challenges/{task_id}`。任务ID只允许安全的单路径段。附件文件名会去除路径、替换不安全字符并在重名时追加编号。

`meta.json`是任务元数据，当前`schema_version`为`1`。写入使用同目录临时文件和原子替换，上一份有效元数据保存在`meta.json.bak`。启动加载时，schema0可重复迁移到1；未知未来schema拒绝加载。

## 字段

|字段|含义|
|---|---|
|`id/name/category/description/target_ip`|任务输入与标识|
|`attachments_dir/attachment_count`|附件位置与数量|
|`status`|兼容状态名|
|`flag/exit_code/error/last_step`|执行结果|
|`writeup_file_name`|安全的任务内WP文件名|
|`container_name/container_kept`|Docker状态关联|
|`opencode_session`|OpenCode继续会话ID|
|`created_at/started_at/finished_at`|UTC生命周期时间|
|`log_size`|当前及归档日志的聚合字节数|

API Key、Provider URL、Prompt、Provider响应体不会写入任务元数据。

## 状态转换

```text
queued -> running -> solved
                  -> completed
                  -> failed
queued/running -> failed(用户停止或不可恢复错误)
```

`solved`表示严格Flag协议通过并允许下载WP。`completed`表示Agent正常结束但在续跑上限内未解出，通常保留容器。`failed`表示执行、超时、取消或基础设施错误。服务重启时，没有保留容器的`queued/running`任务会恢复为排队状态。

## 日志

`logs.txt`达到`CTF_AGENT_LOG_MAX_BYTES`前轮转为`.1`，最多保留`.1`至`.3`。读取API按最旧归档到当前文件合并，任务终端因此仍按时间顺序展示。日志是运行诊断原始记录，不应包含Key；发布问题前仍需人工检查敏感题目内容。

## 迁移原则

迁移必须幂等、保留`.bak`、拒绝未知未来版本并有测试。改变字段语义或状态名之前需要ADR。达到10,000任务、复杂跨任务查询或多进程写入需求时，再评估SQLite迁移。
