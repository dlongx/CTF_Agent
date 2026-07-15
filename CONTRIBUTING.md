# 贡献指南

## 开始之前

1.阅读`AGENTS.md`、`docs/ARCHITECTURE.md`和`docs/DEVELOPMENT.md`。
2.从`main`创建短生命周期分支，一次只处理一个清晰主题。
3.涉及架构、持久化或兼容性变化时，先在`docs/adr/`增加ADR。
4.不要提交`opencode.env`、任务数据、API Key、真实Flag或大型二进制附件。

## 变更要求

- 保持现有API与任务状态兼容，破坏性变化需要迁移路径。
- Go后端是唯一后端；`bridge.py`仅是容器内Agent入口。
- 依赖升级一次只处理一个生态，确保依赖文件和回滚点清晰。
- 新的Skill资料必须注明来源与许可证，并通过本地链接检查。
- 用户可见变化写入`CHANGELOG.md`的`Unreleased`节。
- 完成路线图任务时更新`docs/ROADMAP.md`状态、证据和日期。

## 提交前检查

```powershell
./scripts/check-all.ps1
```

涉及Docker、桥接层、Prompt、Provider或任务生命周期时还要运行:

```powershell
./scripts/smoke-fake-provider.ps1
```

发布前运行`smoke-opencode.bat`验证显式配置的真实Provider。CI或程序均不得静默改用其他Provider。
