---
title: Agent 技能
description: 如果 agent 不会 MCP，/skill 页面会生成一份现成的 SKILL.md，里面已经写好了 hub 地址、管理员 token 和机器列表。
---

**Skill** 页面是不做集成的那条路。它为 agent 生成一份普通 `SKILL.md`：hub 地址、管理员
token、机器列表，以及让 hub 保持健康的规则。没有服务端组件，hub 上也不用装任何东西。

## 生成

![Skill 页面](/screens/skill.webp)

1. 打开控制台并登录。
2. 在导航栏打开 **Skill**。
3. 检查 hub 地址。它默认使用浏览器里的地址，通常就是你要的；反向代理或第二个局域网名字
   可以手动填。
4. 复制或下载 `SKILL.md`。

文件是浏览器通过 hub 的 API 生成的。管理员 token 从已登录的会话里读取，并写进文件。

## 放到 agent 找技能的地方

一个技能一个目录：

```text
~/.claude/skills/abacad/SKILL.md
~/.dsh/skills/abacad/SKILL.md
```

任何加载 skills 目录的 agent 都能用。文件里写了 hub、token，以及每台机器的 `ws_url` 和
VNC 握手需要什么。

## 文件里有什么

- hub 的 origin，以及它是 `http` 还是 `https`。
- 管理员 token，agent 不需要别的。
- 机器表：名称、id、平台、端点、密码要求、状态。
- 连接约定：每台机器是 `/ws/vnc/<id>` 上的 WebSocket，里面是裸 RFB。agent 可以直接打开，
  也可以用 `websocat` 桥到本地 TCP 端口。
- 健康规则：不要探测设备，不要为了看状态而打开会话，记住 hub 是单一入口。

## 当成密码保管

文件里有管理员 token，它能打开所有机器和 hub 设置。不要放进 git，不要放共享盘。如果泄露，
改配置里的 `admin_token` 并重启 hub；所有现有会话和旧 skill 副本都会失效。

## Skill 还是 MCP？

- **MCP** 适合 agent 会说 Streamable HTTP、并且你希望设备列表保持最新，不用重新生成文件。
- **Skill** 适合 agent 只认文件、你想要一个自包含的交接，或者 agent 不能添加 MCP server。

两者最终都到同一个 WebSocket。选一个就好，不需要都接。
