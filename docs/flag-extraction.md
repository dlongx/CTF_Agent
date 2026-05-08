# Flag提取逻辑

平台只在Docker/OpenCode本轮执行结束后提取Flag，不会在运行中的`Observation: assistant output updated:`片段里提前判题。

## 主规则

后端优先读取日志中的最终可读输出块:

```text
Observation: final readable OpenCode output:
```

然后查找严格的两行解出标记:

```text
这道题目已经解出
<exact flag>
```

只有当第一行严格等于`这道题目已经解出`时，才取紧随其后的下一整行作为Flag；该行不能为空。

Flag格式不固定，平台不会再用“最终短文本”、`flag{...}`正则或`Final Flag:`标签作为兜底解题依据。

## WP规则

桥接脚本会在模型确认解出后要求写入`/workspace/<task>-wp.md`，并在日志中输出：

```text
Observation: writeup file=<safe markdown filename>
```

后端只接受安全文件名，并在任务解出后从容器`/workspace`读取该文件保存为下载WP。

## 结果状态

- 提取到合法Flag:任务标记为`solved`，成功任务容器会被移除，并保存WP。
- OpenCode本轮正常退出但没有两行协议:任务暂记为`completed`，不会伪造Flag。
- 如果容器和OpenCode session仍可用，后端会按`CTF_AGENT_AUTO_CONTINUE_ROUNDS`自动续跑同一session；默认自动续跑6轮。
- 续跑耗尽或执行链路硬失败后，任务标记为`failed`，并尽量保留容器供手动继续或排查。
