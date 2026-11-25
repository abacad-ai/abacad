---
title: MCP
description: 通过 POST /mcp 的 Streamable HTTP 把 agent 连到 abacad。唯一的 list_devices 工具会返回每台机器的 WebSocket 和认证要求。
---

abacad 只暴露一个 MCP 工具。这不是需要绕过的限制，而是设计。能驱动 VNC 的 agent 不需要
第二套屏幕 API。

hub 在 **`POST /mcp`** 提供 MCP，使用 **Streamable HTTP**，返回 JSON。会话在 30 分钟无
活动后过期。它使用和网页控制台相同的管理员 token，通过 Bearer 传递：

```text
Authorization: Bearer <admin token>
```

## 连接客户端

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer <admin token>" \
  abacad http://192.168.1.10:8787/mcp
```

或者在 JSON 客户端配置里：

```json
{
  "mcpServers": {
    "abacad": {
      "type": "http",
      "url": "http://192.168.1.10:8787/mcp",
      "headers": { "Authorization": "Bearer <admin token>" }
    }
  }
}
```

如果 hub 在 TLS 后面，用 `https://…/mcp`。

## `list_devices`

输入：无。

```json
{
  "devices": [
    {
      "id": "mac-mini",
      "name": "Mac mini",
      "platform": "macos",
      "online": true,
      "known": true,
      "status": "online",
      "mode": "gateway",
      "ws_url": "ws://192.168.1.10:8787/ws/vnc/mac-mini",
      "auth": "none",
      "sign_in": "ard",
      "username": "iloahz",
      "browser_url": "http://192.168.1.10:8787/vnc/mac-mini"
    }
  ]
}
```

值得注意的字段：

- `ws_url` —— 承载裸 RFB 的 WebSocket。`direct` 设备为空。
- `auth` —— `none` 表示 hub 替你登录（gateway）；`device` 表示客户端要回答设备自己的 VNC
  密码（l4）；`direct` 设备还会带 `direct_host`/`direct_port`，只做目录。
- `sign_in` —— `vnc` 或 `ard`（macOS 账户）。这是 hub 在上游使用的登录方式。
- `status` —— `online`、`offline` 或 `unknown`（从未见过）。状态十分钟后过期；它不是实时
  探测。
- `last_error` —— 最近一次失败，如果有。

## 打开流

agent 用同一个管理员 token 打开 `ws_url`，可以放在 `Authorization: Bearer` 头里，也可以
放在 `?token=` 里（给只接受 URL 的客户端）：

```text
ws://192.168.1.10:8787/ws/vnc/mac-mini
Authorization: Bearer <admin token>
```

如果 agent 有 WebSocket VNC 客户端，这就是全部集成。如果客户端只会 TCP，先桥到本地端口：

```bash
websocat --binary -H="Authorization: Bearer <admin token>" \
  tcp-l:127.0.0.1:5900 ws://192.168.1.10:8787/ws/vnc/mac-mini &

vncdotool -s 127.0.0.1::5900 capture screen.png
```

然后在 `127.0.0.1:5900` 上使用任何 VNC 客户端 —— `vncdotool`、TigerVNC、noVNC、截图工具，
或者 agent 自己的 VNC 实现。

## 不用 MCP？用 skill

不会 MCP 的 agent 仍然可以用 hub。**Skill** 页面会生成一份现成的 `SKILL.md`，把 hub 地址、
管理员 token 和机器列表都写进去。它就是一个普通文件；没有服务端集成。见
[agent 技能](/zh/docs/agents/skill/)。

## 注意

- **一个 token 打开所有机器。** 3.0 没有每设备 token。把 token 当成整个 hub 的密码。
- **hub 不是审批层。** `gateway` 模式下它替客户端登录设备，不会要求人逐步批准每个动作。
  如果你需要审批，请在 agent 外面加自己的策略。
- **每个查看者一个会话。** 有些 VNC 服务允许多个查看者，有些不允。浏览器和 agent 冲突时，
  一次用一个。
- **WebSocket 里是 RFB，不是新协议。** 同一个流既能给 noVNC，也能给任何标准 VNC 客户端。
  见 [WebSocket 与 VNC](/zh/docs/agents/websocket/)。
