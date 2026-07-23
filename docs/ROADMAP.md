# 路线图

本文件是任务状态的唯一来源。状态使用`done`、`in_progress`、`blocked`、`planned`。证据必须是可重复命令、测试名称、工作流或文档路径；提交列在尚未提交时写`working-tree`。

最后更新时间:2026-07-20

|ID|状态|完成证据|关联提交|最后更新|
|---|---|---|---|---|
|P0-01|done|Go1.25.12/1.26.5测试、vet和构建通过；`govulncheck`可达漏洞0项|working-tree|2026-07-13|
|P0-02|done|`TestRunTaskStopsAtAutoContinueLimit`、并发继续与队列测试|working-tree|2026-07-13|
|P0-03|done|默认`0s/0s/0s`不限时；`TestLoadConfigUsesSingleTaskUnlimitedDefaults`、`test_read_config_defaults_to_unlimited_timeouts`及显式超时分类测试|working-tree|2026-07-20|
|P0-04|done|`test_bridge.py`配置、Prompt文件和脱敏测试；假Provider真实容器烟测|working-tree|2026-07-13|
|P0-05|done|`/health`、`/ready`、`/api/settings/provider/test`及诊断测试|working-tree|2026-07-13|
|P0-06|done|`smoke-opencode.bat`使用真实截止时间、可靠休眠、预检和失败清理|working-tree|2026-07-13|
|P0-07|done|真实失败任务cgroup记录`oom_kill=1`；`TestDockerOOMHelpers`、`TestRunnerFailureMessageReportsOOM`；reverse镜像使用`unar`成功解出RAR5附件|working-tree|2026-07-20|
|P0-08|done|假Provider烟测设置`CTF_AGENT_CONTAINER_RETENTION=0s`；`TestContainerRetentionAcceptsZero`；全量检查与烟测通过|working-tree|2026-07-20|
|P0-09|done|真实超时任务遗留进程复现；`test_process_tree_helpers_and_timeout_exit_codes`；Docker `--init`及单轮/空闲超时分类测试|working-tree|2026-07-20|
|P0-10|done|`TestAutoDockerResourceLimits`；Go配置、`start-dev.bat`及示例环境均默认单任务独占；真实容器提升至6720MiB/15CPU|working-tree|2026-07-20|
|P1-01|done|`.github/workflows/ci.yml`与`docker.yml`；Go1.26.5安全扫描；Windows全新检出保持Go文件LF|working-tree|2026-07-15|
|P1-02|done|README、AGENTS、MIT、架构、开发、运维、API、数据模型、ADR和来源清单|working-tree|2026-07-13|
|P1-03|done|schema1、原子写入、`.bak`恢复、10MiB×4日志、备份恢复脚本及Store测试|working-tree|2026-07-13|
|P1-04|done|固定基础Digest/OpenCode1.17.18/直接依赖；8个镜像本机重建通过；PR选择受影响题型、月度构建全部题型|working-tree|2026-07-13|
|P1-05|done|7个运行镜像UID1000、工具导入和`pip check`通过；24h清理、孤儿回收和磁盘显示已测试|working-tree|2026-07-13|
|P1-06|done|6个原生`SKILL.md`目录、取消截断、代码块排除及`820 local links checked`|working-tree|2026-07-15|
|P1-07|done|JSON`slog`、任务原始日志、队列/运行数/Docker清理诊断|working-tree|2026-07-13|
|P1-08|done|ES Modules、超时/重连/无障碍状态；1280x720、820x820和390x844首页/详情/Docker页无横向溢出、无控制台错误；手机详情元数据264px、终端320px|working-tree|2026-07-14|
|P1-09|done|Windows Go覆盖率71.6%、Linux72.0%；Python桥接核心85.4%；Go竞态与WebSocket/并发/恢复错误分支通过|working-tree|2026-07-14|
|P1-10|done|`staticcheck ./...`零问题；重复文件哈希零组；净删2个文件、25行源码和256行失效文档；全量检查、假Provider烟测及三页浏览器复核通过|working-tree|2026-07-13|

## 当前发布门禁

|检查|状态|证据|
|---|---|---|
|确定性检查|done|`./scripts/check-all.ps1 -Vulnerability`，可达漏洞0项|
|Linux竞态与双Go版本|done|Go1.25.12/1.26.5容器测试；Go1.26`-race`通过|
|假Provider实际任务|done|misc镜像任务解出`flag{ctf_agent_smoke_ok}`并生成WP|
|本地依赖就绪|done|`/ready`共10项检查全部通过|
|真实Provider烟测|blocked|2026-07-13显式`/models`检查20秒超时；发布前必须恢复且完成真实任务烟测|

## 长期治理

|周期|任务|
|---|---|
|每月|依赖更新、`govulncheck`、Docker漏洞/SBOM检查、全类别镜像构建|
|每季度|全镜像无缓存重建、备份恢复演练、Markdown链接检查、保留策略复核|
|每次发布|SemVer、CHANGELOG、全CI、假Provider及真实Provider烟测、密钥泄露审计|

## 暂不实施

Vue、SQLite、微服务、RBAC和公网多租户不在当前路线图。触发阈值以[架构文档](ARCHITECTURE.md)为准，达到阈值后先写ADR。
