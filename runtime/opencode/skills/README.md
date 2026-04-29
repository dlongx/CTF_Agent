# OpenCode内置CTF Skills

本目录保存容器内Agent可注入的CTF解题Skill。当前已内置6个主题型：

```text
crypto.md
web.md
pwn.md
reverse.md
forensics.md
misc.md
```

`reference/`目录保存每个题型的详细参考资料。后续桥接脚本可以按题型读取主Skill，并在需要时把相关参考文件名或内容加入Prompt。

推荐加载规则：

```text
题型为crypto     -> crypto.md
题型为web        -> web.md
题型为pwn        -> pwn.md
题型为reverse    -> reverse.md
题型为forensics  -> forensics.md
其他或混合题型    -> misc.md
```
