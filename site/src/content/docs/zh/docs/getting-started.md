---
title: 快速开始
description: 跑起 abacad，用管理员 token 登录，添加第一台 VNC 设备，在浏览器里打开它，并让 agent 连上同一个 hub。
---

从零到“能在浏览器里打开一台机器，并把它交给 agent”的最短路径。这里假设那台机器已经
跑着 VNC 服务；如果还没有，先看 [平台指南](/zh/docs/devices/macos/)。

## 你需要什么

- 一台一直开着的机器 —— Mac mini、NAS、树莓派、小型 Linux 主机。这是 **hub**。
- Docker，或者 Go 1.27+ 来构建和运行二进制。只有从源码构建前端时才需要 Node；镜像已经
  包含前端，从源码构建的二进制也会内嵌它。
- 一台跑着 VNC 服务的设备，能从 hub 的局域网或 VPN 地址访问到。

## 1. 跑起 hub

```bash
docker run -d --name abacad --network host \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest

docker logs abacad
```

Linux 上强烈建议 `--network host`：这样 hub 日志里能看到真实的客户端 IP。macOS 和
Windows 的 Docker Desktop 请改用 [bridge 网络](/zh/docs/install/#bridge-networking-instead-of-host)。

日志会打印 URL 和生成的管理员 token：

```text
  abacad is running
  → http://192.168.1.10:8787
  admin token: 3f2c...
```

token 是 64 个十六进制字符。网页登录、API、MCP 和每台设备的 WebSocket 都用它。当成密码
保管。

想用二进制？见 [安装](/zh/docs/install/#the-binary)。

## 2. 打开控制台

在浏览器里打开日志里的 URL，粘贴管理员 token。控制台会停在设备栏。如果你在另一台机器
上，确保 hub 的端口可达 —— VPN 或 SSH 隧道就够了，不需要暴露到公网。

## 3. 添加设备

点 **添加设备**，填：

- **名称** —— 你在设备栏里认得出的名字，比如 `Mac mini`。
- **Host** —— **hub** 能访问到的地址：局域网 IP、VPN 地址、DNS 名，或者转发到本地的端口。
  它不是相对于你的浏览器，而是相对于 hub。
- **端口** —— VNC 服务的端口，通常是 `5900`。
- **登录方式** —— **VNC 密码**，或者 Mac 上的 **macOS 账户**。Mac 请用账户方式；密码方式
  没有用户身份，可能落到登录窗口。
- **平台** —— 用于图标和状态提示。
- **模式** —— 没有特别理由就留在 `gateway`。

保存。设备栏里就会出现这台机器。

## 4. 打开它

在设备栏里选中这台机器，画面会在旁边的面板里打开。第一次成功会话会把设备标成
**在线**；在那之前是 **未见过**，因为 abacad 不做探测。

画面是 noVNC 自己的应用。它的控制栏有键盘、剪贴板、全屏、缩放和重连。大多数浏览器的
剪贴板需要 HTTPS；见 [TLS 与反向代理](/zh/docs/install/#tls-and-the-reverse-proxy)。

## 5. 把同一个 hub 交给 agent

agent 通过 MCP 访问设备，端点是 `POST /mcp`，用同一个管理员 token：

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer <admin token>" \
  abacad http://192.168.1.10:8787/mcp
```

唯一的工具 `list_devices` 会返回每台机器的 `ws_url`；agent 用 WebSocket VNC 客户端打开它。
如果 agent 不会 MCP，hub 也可以生成一份现成的 `SKILL.md`：

1. 登录后打开控制台里的 **Skill**。
2. 复制或下载文件。
3. 放到 agent 找技能的位置，例如 `~/.claude/skills/abacad/SKILL.md`。

文件里有管理员 token，不要放进 git。

## 下一步

- [安装](/zh/docs/install/) — Docker、二进制、TLS、升级和配置文件。
- [macOS](/zh/docs/devices/macos/)、[Windows](/zh/docs/devices/windows/)、[Linux](/zh/docs/devices/linux/)、
  [Android](/zh/docs/devices/android/)、[iOS](/zh/docs/devices/ios/) — 各平台服务端怎么配。
- [MCP](/zh/docs/agents/mcp/) 和 [agent 技能](/zh/docs/agents/skill/) — agent 连接的两种方式。
- [安全](/zh/docs/operations/security/) — 在把 hub 暴露到局域网之外以前先读。

