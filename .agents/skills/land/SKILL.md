---
name: land
description: >-
  仅在用户明确请求落地或合并CTF_Agent项目变更时使用，通过GitHub PR将已验证的变更合并到origin/main。仅审查、准备、检查通过或安装本技能不构成落地请求。
disable-model-invocation: true
metadata:
  delta-action: land
---

# CTF_Agent变更落地

本技能的显式调用即为落地请求，不要再次索要相同的合并许可。完成必要准备、验证、PR合并和远端结果确认；只提交、推送或创建PR不算完成。遇到真正的阻塞时明确报告“尚未落地”及原因。

来源：根据本仓库的AGENTS.md、CONTRIBUTING.md、实际检查脚本和GitHub工作流编写，属于项目原创维护资料，遵循仓库LICENSE中的MIT许可证。

## 1. 确认范围与目标

- 阅读当前AGENTS.md、CONTRIBUTING.md、README.md、docs/ARCHITECTURE.md、docs/DEVELOPMENT.md，以及变更路径适用的嵌套指令。
- 使用`git --no-optional-locks status --short`、`git branch -avv`、`git remote -v`和分支差异确认工作区、提交范围与目标。保留无关的已暂存和未暂存修改，不自动stash、清理或覆盖用户文件；范围不明时先询问。
- 本流程只适用于CTF_Agent。核实origin仍指向用户预期的GitHub仓库，默认目标为origin/main；local是本机回链，不用于发布。目标改变时停止核实，不猜测。
- 检查Git、GitHub CLI、PowerShell7、Go、Python、Node和按需使用的Docker；运行时版本以go.mod及.github/workflows/ci.yml为准。调查时环境为Go1.26.5、Python3.12、Node24、PowerShell7，不能把这些已安装状态当作永久保证。
- 通过GitHub CLI确认认证可用，避免输出令牌或完整环境。读取当前目标分支保护、rulesets、仓库合并设置和PR审核状态；API无法验证要求时停止，不把权限错误视为没有规则。

## 2. 准备可审查的变更

- 复用当前主题分支及其面向main的开放PR；若没有主题分支，按CONTRIBUTING.md从最新main建立短生命周期分支，保留本次改动。不要直接推送main。
- 核对当前贡献规范：架构、持久化或兼容性变化需要ADR；破坏性变化需要迁移路径；依赖升级一次只处理一个生态。保持Go唯一后端及现有API、任务状态约定。
- 按AGENTS.md和CONTRIBUTING.md更新适用的CHANGELOG.md未发布节及docs/ROADMAP.md；完成路线图任务必须附日期和真实测试或烟测证据，不杜撰通过结果。新Skill资料注明来源、许可证，并参加链接检查。
- 不提交opencode.env、密钥、真实Flag、任务数据或大型二进制附件。审阅暂存差异，按明确文件路径暂存，不用无范围的批量添加。
- 使用简洁、描述主题的提交消息，遵循近期feat:、fix:、docs:等风格；遵守当前存在的签名或其他贡献要求，不关闭签名配置来绕过失败。提交使用`git -c core.editor=true commit -m "明确的提交说明"`，不启动交互编辑器。
- 若需更新基线，fetch origin后把origin/main合并到主题分支，使用`git -c core.editor=true merge --no-edit origin/main`，不重写已发布历史。意图明确的冲突自动解决并审阅结果；有歧义、可能覆盖无关工作或涉及不安全变化时暂停并说明冲突。不要使用整侧覆盖作为通用解决方案。
- 对检查暴露的问题，只在本次明确范围内修复；大范围或语义不明的问题先报告阻塞。任何代码修改、冲突解决或基线更新后，重新执行受影响验证。

## 3. 验证待合并版本

所有适用的必要检查必须在落地前通过，并对应实际待合并内容。等待中、失败、缺失、无法验证或旧提交的成功结果均不能算通过。

- 在仓库根目录用PowerShell7运行`./scripts/check-all.ps1`。来源：scripts/check-all.ps1的无参数入口及.github/workflows/ci.yml中Run deterministic checks步骤。它包含格式、依赖一致性、Go测试及覆盖率、vet、构建、Python测试及覆盖率、JavaScript和Markdown链接检查，不以单个单测替代。
- 涉及Docker、桥接层、Prompt、Provider或任务生命周期时运行`./scripts/smoke-fake-provider.ps1`。来源：CONTRIBUTING.md的适用条件、scripts/smoke-fake-provider.ps1的无参数入口和.github/workflows/docker.yml的Fake-provider task smoke步骤。先确认Docker可用、测试端口无冲突、镜像由待测版本构建。需要构建时使用.github/workflows/docker.yml中对应共享镜像、受影响题型镜像的实际命令，不凭记忆改参数；默认烟测需要misc镜像。
- 烟测脚本使用隔离临时数据，关闭孤儿清理，并在成功后清理自身临时目录。若运行环境要求对工作区外临时写入或清理单独授权，先取得该范围授权；不要删除用户原有目录或容器。缺少Docker或镜像不能报告本地烟测通过，应停止补齐条件。
- GitHub CI必须覆盖当前工作流要求的Ubuntu/Windows与Go1.25/1.26矩阵，以及Linux竞态和漏洞检查。来源：.github/workflows/ci.yml。Linux准确命令为`go test -count=1 -race ./...`及`go run golang.org/x/vuln/cmd/govulncheck@latest ./...`；本机没有CGO/C编译器时依赖对应Linux CI证据，不声称本机已执行或以普通测试替代。
- Docker工作流按.github/workflows/docker.yml的路径条件和镜像选择逻辑适用。适用时所有相应构建、验证及烟测必须通过，不因为分支未设保护而忽略失败。曾观察到的PR编号、失败状态只是历史信息，每次都重新读取当前结果。
- 发布前必须有显式配置的真实Provider烟测证据；普通合并不自动扩大为发布。来源：CONTRIBUTING.md及smoke-opencode.bat。该批处理无参数入口依赖已有服务，并用curl参数传递访问令牌；如使用令牌会违反密钥不得进入进程参数的约束，不直接调用，报告阻塞并先取得安全执行方案。真实Provider未配置或失败时禁止静默切换，禁止假报发布验证完成。
- 检查工具缺失或本地检查不能完成时，报告准确限制并停止补齐条件；远端矩阵可承担上述明确允许的跨平台验证，但不能用局部成功覆盖其他必要检查。

## 4. 发布分支并完成PR合并

- 确认只包含用户授权范围。向既有origin发布该主题分支，使用普通非强制push；不向local发布、不强推、不绕过审核或修改保护规则。若发现待发布内容可能为私有代码且未获对该目的地的发布授权，先请求授权。
- 用GitHub CLI按head与base查找并复用PR，没有时创建面向main的PR。使用非交互参数提供标题和正文，正文说明变更、验证、适用ADR及已知限制；若当前规范要求人工撰写的提交材料，应获取原文而非用批准替代人工撰写。
- 读取PR当前head SHA、base、检查结果、reviewDecision和可合并状态，确保与已验证内容一致。等待所有适用检查完成，满足当前审核、讨论及目标设置；超时或失败报告阻塞，不能启动检查后直接合并。
- 默认使用merge commit，与项目近期实践一致，但这不是不可变的仓库政策。执行时确认仓库允许该方式；若当前规则要求队列或其他策略，遵循明确规则，无法确认时暂停，不擅自绕过或改策略。
- 普通允许merge commit的路径使用`gh pr merge <PR编号> --merge --match-head-commit <已验证的完整HEAD SHA>`，替换为本次实际值，不使用管理员绕过。不要默认删除分支，不执行交互rebase。若head变化，重新验证后再尝试；若队列接管，等待真正合并而非将入队视为成功。

## 5. 核验结果

- 重新查询PR，必须为MERGED，记录实际merge commit、目标分支和PR链接。
- fetch origin并确认实际merge commit是origin/main的祖先，例如使用`git merge-base --is-ancestor <实际merge commit> origin/main`并检查退出码。不只依据CLI“已提交请求”判断成功。
- 复查工作区，确认无关修改仍被保留。不要为同步结果重置用户分支、删除分支、执行容器清理或生成发布。
- 用中文总结落地目标、PR与提交、实际验证证据及遗留限制；若合并未完成，明确说明尚未落地及阻塞点。
