# ADR-0003:OpenCode作为容器执行边界

- 状态:Accepted
- 日期:2026-07-13

## 背景

CTF附件和解题工具不可信且依赖复杂。Go服务不应直接运行工具或承担模型协议细节。

## 决策

每个任务使用独立Docker容器，`runtime/opencode/bridge.py`是容器内唯一Agent入口，OpenCode CLI负责Provider交互。Key通过一次`docker exec`环境和OpenCode环境引用传递，长Prompt使用临时文件。

## 后果

获得清晰隔离和题型镜像复用，但依赖Docker可用性和镜像体积。发布验证必须包括真实容器烟测；Provider故障必须显式报告，禁止静默切换。
