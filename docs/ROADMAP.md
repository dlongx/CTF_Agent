# 路线图

本文件是任务状态的唯一来源。状态使用`done`、`in_progress`、`blocked`、`planned`。证据必须是可重复命令、测试名称、工作流或文档路径；提交列在尚未提交时写`working-tree`。

最后更新时间:2026-10-05

|ID|状态|完成证据|关联提交|最后更新|
|---|---|---|---|---|
|P0-01|done|Go1.26.6统一检查、Linux竞态和漏洞扫描通过；Go1.25.12兼容测试与构建通过；依赖修复及完整证据见下文|working-tree|2026-10-05|
|P0-02|done|`TestRunTaskStopsAtAutoContinueLimit`、并发继续与队列测试|working-tree|2026-07-13|
|P0-03|done|默认`0s/0s/0s`不限时；`TestLoadConfigUsesSingleTaskUnlimitedDefaults`、`test_read_config_defaults_to_unlimited_timeouts`及显式超时分类测试|working-tree|2026-07-20|
|P0-04|done|`test_bridge.py`配置、Prompt文件和脱敏测试；假Provider真实容器烟测|working-tree|2026-07-13|
|P0-05|done|`/health`、`/ready`、`/api/settings/provider/test`及诊断测试|working-tree|2026-07-13|
|P0-06|done|`smoke-opencode.bat`使用真实截止时间、可靠休眠、预检和失败清理|working-tree|2026-07-13|
|P0-07|done|真实失败任务cgroup记录`oom_kill=1`；`TestDockerOOMHelpers`、`TestRunnerFailureMessageReportsOOM`；reverse镜像使用`unar`成功解出RAR5附件|working-tree|2026-07-20|
|P0-08|done|假Provider烟测设置`CTF_AGENT_CONTAINER_RETENTION=0s`；`TestContainerRetentionAcceptsZero`；全量检查与烟测通过|working-tree|2026-07-20|
|P0-09|done|真实超时任务遗留进程复现；`test_process_tree_helpers_and_timeout_exit_codes`；Docker `--init`及单轮/空闲超时分类测试|working-tree|2026-07-20|
|P0-10|done|`TestAutoDockerResourceLimits`；Go配置、`start-dev.bat`及示例环境均默认单任务独占；真实容器提升至6720MiB/15CPU|working-tree|2026-07-20|
|P1-01|done|`.github/workflows/ci.yml`竞态与安全扫描固定Go1.26.6，本地同版本检查通过；Docker工作流烟测仍为发布阻塞，见下文|working-tree|2026-10-05|
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
|确定性检查|done|2026-10-05 Windows Go1.26.6执行`./scripts/check-all.ps1 -Vulnerability`通过；可达漏洞0项，导入包漏洞0项；模块级剩余4项不可达报告，见下文|
|Linux竞态与双Go版本|done|2026-10-05 Linux Go1.26.6执行`go test -count=1 -race ./...`通过；Go1.25.12执行tidy一致性、测试和构建通过|
|假Provider实际任务|blocked|[Docker失败工作流](https://github.com/dlongx/CTF_Agent/actions/runs/29988751576)：等待300秒任务仍为`running`，日志停在启动OpenCode，未取得预期Flag；历史本机成功不能替代当前失败，本次未修复、未重跑该烟测|
|本地依赖就绪|done|`/ready`共10项检查全部通过|
|真实Provider烟测|blocked|2026-07-13显式`/models`检查20秒超时；发布前必须恢复且完成真实任务烟测|

### 2026-10-05工具链与依赖安全验证

- 基线：Windows Go1.26.5运行`./scripts/check-all.ps1`通过。
- 版本选择：`x/net`v0.57.0要求Go1.25.0，覆盖扫描列出的HTTP/2、IDNA、HTML和DNS修复；未选要求Go1.26.0的最新版v0.59.0，以保留Go1.25兼容性。
- 依赖图：按`x/net`v0.57.0的要求将`x/crypto`升级到v0.54.0、`x/text`升级到v0.40.0；`x/sys`已有v0.47.0，无需修改。`x/term`未进入项目整理后的依赖列表。`go.mod`保留`go 1.25.0`，`go.sum`由`go mod tidy`同步。
- Windows验证：设置进程环境`GOTOOLCHAIN=go1.26.6`后执行`./scripts/check-all.ps1 -Vulnerability`，退出码0；Go覆盖率71.1%，Python单测17项通过、桥接核心覆盖率83.3%，820个本地Markdown链接通过。
- Linux验证：以只读方式将工作区挂载到`golang:1.26-bookworm`的`/src`，设置`GOTOOLCHAIN=go1.26.6`、`CGO_ENABLED=1`，在`/src`执行`go version && go test -count=1 -race ./... && go run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./...`，确认实际工具链为Go1.26.6、退出码0；本次扫描器版本v1.8.0。
- 兼容验证：同样只读挂载到`golang:1.25-bookworm`，设置`GOTOOLCHAIN=local`，执行`go version && go mod tidy -diff && go test -count=1 ./... && go build -o /tmp/go-server ./cmd/go-server`；实际Go1.25.12，退出码0。
- 下载环境：默认Go代理连接超时，以上更新后检查临时使用进程环境`GOPROXY=https://goproxy.cn`，未修改全局配置或关闭校验。
- 扫描边界：Windows/Linux均为可达漏洞0项、导入包漏洞0项，并非整个依赖图没有漏洞。仍有`x/crypto`模块级GO-2026-6355、GO-2026-6354、GO-2026-6303（SSH）及GO-2026-5932（OpenPGP）报告，当前代码未导入对应漏洞包；后续引入这些包前必须重新评估。
- 本次证据为本地检查，不代表远端CI已通过；Docker假Provider及真实Provider烟测阻塞均未解除。

2026-10-05落地记录：新增项目级`.agents/skills/land/SKILL.md`；本地`./scripts/check-all.ps1`通过，Go覆盖率71.1%、Python桥接核心覆盖率83.0%，17项Python测试及820个本地Markdown链接检查通过。操作者明确接受PR #9假Provider烟测失败风险，授权本次整条分支直接推送到`main`；该例外不表示烟测恢复或发布门禁通过，也不修改今后的常规落地要求。

## 长期治理

|周期|任务|
|---|---|
|每月|依赖更新、`govulncheck`、Docker漏洞/SBOM检查、全类别镜像构建|
|每季度|全镜像无缓存重建、备份恢复演练、Markdown链接检查、保留策略复核|
|每次发布|SemVer、CHANGELOG、全CI、假Provider及真实Provider烟测、密钥泄露审计|

## 暂不实施

Vue、SQLite、微服务、RBAC和公网多租户不在当前路线图。触发阈值以[架构文档](ARCHITECTURE.md)为准，达到阈值后先写ADR。
