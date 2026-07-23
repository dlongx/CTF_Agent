# Changelog

本项目遵循[Semantic Versioning](https://semver.org/)。

## Unreleased

### Added

- `/ready`依赖就绪检查和显式Provider连通性测试。
- 假Provider端到端烟测、GitHub Actions、备份恢复和Markdown链接检查。
- 元数据版本、备份恢复、日志轮转、容器保留清理和结构化运行日志。
- OpenCode原生Skill目录及原生前端ES Modules。

### Changed

- 最低Go版本升级到1.25，Gin升级到1.12.0。
- 默认改为单任务独占；容器自动使用除1个CPU和至少1GiB内存外的Docker可用资源。
- `quic-go`升级到0.59.1，修复可达漏洞GO-2026-5676。
- CI漏洞扫描固定使用Go1.26.5；Python基础镜像在工具链完成兼容验证前保持3.12系列更新。
- 默认关闭任务总时长、单轮时长和无输出时长限制，仍可通过环境变量显式恢复边界。
- 自动续跑受6轮上限和全局Worker并发约束，到达上限后进入`completed`。
- Docker镜像固定基础镜像Digest、OpenCode和直接依赖版本，并以非root用户运行。
- 前端任务与容器状态合并回唯一入口模块，避免为4行私有状态增加独立请求和文件。
- 首页标题、Provider状态、操作区和工作区改为分级响应式布局，窄屏页面使用自然纵向阅读。

### Removed

- 删除无调用点的`ContinueTask`兼容包装、手写整数转换和Go内置`max`替代实现。
- 删除与当前密钥隔离、超时策略冲突且无文档入口的旧OpenCode权限说明。

### Fixed

- reverse镜像补充`unar`以处理`7z`不支持的RAR方法；任务执行器通过cgroup增量识别OOM并返回当前内存上限和恢复提示。
- 假Provider烟测关闭全局容器保留清理，避免临时数据目录把真实服务的保留容器误判为孤儿并删除。
- Bridge超时时递归终止脱离OpenCode进程组的工具子进程，并以独立退出码区分单轮超时和无输出超时；新容器启用Docker init回收僵尸进程。
- 修复Windows全新检出后CRLF导致`gofmt`失败，以及Linux把代码块内容误判为Markdown链接的问题。
- 修复reverse镜像的Z3、Pillow和Unicorn依赖冲突，以及web镜像的Wfuzz无效元数据。
- 修复登录shell丢失CTF工具PATH和移动端Provider面板被固定高度撑开的问题。
- Windows并行文件测试使用有界重试清理临时目录，避免安全扫描导致的瞬时失败。
- 修复低高度桌面表单内滚、移动任务详情内容被压扁、Docker卡片横向溢出及长错误拉伸顶部的问题。

### Security

- OpenCode配置改用环境变量内容和Key环境引用，不再把密钥写入工作区。
- 长Prompt改用权限受限的临时文件，不再出现在进程参数中。
- WebSocket增加握手、来源、帧大小、掩码、超时和断线处理。
