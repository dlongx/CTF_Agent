# 第三方来源与许可证

本文件是来源清单，不替代各上游项目的许可证。发布镜像前应使用镜像SBOM复核最终传递依赖。

|组件或资料|用途|来源|许可证|
|---|---|---|---|
|Go|服务运行时|[go.dev](https://go.dev/)|BSD-3-Clause|
|Gin|HTTP路由与中间件|[gin-gonic/gin](https://github.com/gin-gonic/gin)|MIT|
|OpenCode|容器内Agent CLI|[anomalyco/opencode](https://github.com/anomalyco/opencode)|MIT|
|AI SDK OpenAI Compatible|OpenAI兼容Provider适配|[vercel/ai](https://github.com/vercel/ai)|Apache-2.0|
|AI SDK Anthropic|Anthropic Provider适配|[vercel/ai](https://github.com/vercel/ai)|Apache-2.0|
|Python基础镜像|Docker运行时|[docker-library/python](https://github.com/docker-library/python)|PSF及镜像内组件各自许可证|
|GTFOBins链接|Skill外部参考|[GTFOBins](https://gtfobins.github.io/)|GPL-3.0，项目仅链接|
|dCode链接|Skill外部工具参考|[dCode](https://www.dcode.fr/)|第三方网站条款，项目仅链接|

`runtime/opencode/skills/*/SKILL.md`声明MIT。`references/`中的当前内容随本仓库以MIT提供；新增或移植第三方文本前必须确认再分发许可，并在本文件增加精确来源。Docker镜像还包含APT、pip、npm和RubyGems软件，其许可证可通过各包管理器和镜像SBOM查询。
