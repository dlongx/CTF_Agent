# API

默认地址`http://127.0.0.1:8000`。配置访问令牌后，客户端可使用`Authorization: Bearer <token>`、`X-CTF-Agent-Token`或HTTP Basic Auth密码。

## 健康与诊断

|方法|路径|成功|说明|
|---|---|---:|---|
|GET|`/health`|200|进程存活，不检查依赖|
|GET|`/ready`|200/503|数据、Provider、Docker和镜像就绪状态|
|GET|`/api/settings/provider`|200|Provider脱敏配置状态|
|POST|`/api/settings/provider`|200|请求`{"format":"openai-compatible"}`切换格式|
|POST|`/api/settings/provider/test`|200/400/502|显式测试指定格式的`/models`|

`/ready`响应只包含`ok`、各依赖的`ok/reason`和`checked_at`。Provider测试响应包含`format/ok/latency_ms/error_code/checked_at`，不会返回URL、Key和响应体。

## 任务

|方法|路径|说明|
|---|---|---|
|GET|`/api/tasks`|任务列表|
|POST|`/api/tasks`|multipart创建任务，返回202|
|GET|`/api/tasks/{id}`|任务详情|
|GET|`/api/tasks/{id}/logs?tail=N`|合并后的轮转日志，可按末尾字节截取|
|GET|`/api/tasks/{id}/writeup`|仅已解出任务可下载WP|
|POST|`/api/tasks/{id}/messages`|`{"message":"..."}`继续同一session|
|POST|`/api/tasks/{id}/hints`|兼容接口，请求`{"hint":"..."}`|
|POST|`/api/tasks/{id}/stop`|取消运行并关闭容器|
|POST|`/api/tasks/{id}/container/close`|关闭保留容器|

创建任务字段:`name`最多200字符，`type`为6种支持题型，`description`最多20,000字符，`target_ip`最多512字符，附件字段名为`attachments`。multipart总大小和单附件上限均为256MiB。

继续请求仅允许容器已保留且存在OpenCode session的非运行任务。同一任务已有继续请求时返回409，全局队列满返回429。

## 实时流

- `GET /api/events`:SSE任务变化通知。
- `GET /ws/tasks/{id}/logs`:WebSocket任务日志。

WebSocket要求版本13、标准掩码、同源或显式允许Origin。浏览器不能自定义Authorization头时可先通过同源认证页面建立会话；当前Basic Auth由浏览器处理。

## 通用错误

JSON错误格式为`{"detail":"..."}`。常见状态:400输入或状态不合法、401未认证、404资源不存在、409同任务冲突、413上传过大、429队列满、502Provider诊断失败、503未就绪。
