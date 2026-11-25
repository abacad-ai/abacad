---
title: 排错
description: "修常见的 abacad 问题：设备一直是未见过、认证失败、TigerVNC 黑名单、代理和 WebSocket 问题，以及过期的 iOS 会话。"
---

先看 hub 的日志。容器加 `-v`（或二进制加 `-v`），然后打开设备看发生了什么。大多数失败是
五件事之一：hub 到不了主机、端口没开、凭据不对、VNC 服务选了另一个会话，或者代理没有
升级 WebSocket。

## 设备一直是“未见过”

**未见过** 表示从来没有认证成功的会话到达设备。abacad 不探测，所以这不是健康检查结果。
按顺序检查：

1. **从 hub 能到主机吗？** 不是从你的笔记本 —— 是从 hub。host 字段由 hub 解析。
2. **VNC 服务监听的网卡 hub 能到吗？** 在设备上：
   `ss -ltnp | grep 5900`（Linux）或 `lsof -nP -iTCP:5900`（macOS）。`127.0.0.1` 从 hub 到不了。
3. **防火墙规则对吗？** 放行 hub 的 IP，不是 any。
4. **模式对吗？** `direct` 设备只做目录，没有浏览器会话。
5. **会话认证成功了吗？** 打开一次并看日志；只有握手成功设备才会变成在线。

不要用 `nc`、负载均衡或监控探测 TigerVNC 端口。一个在认证前关闭的 TCP 连接会被算作安全
失败。

## 认证失败

- **VNC 密码设备：** 重新输入密码。经典 VNC 密码只有前八个字符有效；更长的密码会被截断，
  而不是拒绝，所以前八位里打错一个字是常见原因。
- **macOS 账户：** 用账户短名（`whoami`），不是显示名；用登录密码，不是旧的 VNC 密码。如果
  Mac 开了 **远程管理**，账户必须在 **允许访问** 列表里，并有 Observe/Control。
- **macOS 报 `Authentication or authorization failure`：** DH 交换成功了，Mac 拒绝了账户。
  检查远程管理的访问列表，不是密码。
- **`macOS: switch Sign-in to "macOS account"`：** 设备还配着旧的 VNC 密码方式。编辑并切换。

## `Too many security failures`

TigerVNC 在反复出现认证前关闭的连接后会拉黑客户端 IP，计数器不衰减。修复方法是重启 VNC
服务，清掉内存里的计数：

```bash
sudo systemctl restart tigervncserver@:2.service   # Linux
# 或者重启 Windows 上的 TigerVNC 服务
```

只调高 `BlacklistThreshold` 不会清除已有的拉黑。重启后停掉探测端口的工具 —— 包括监控、
负载均衡，或者一直循环重连的浏览器标签页。

## 控制台打不开

- **401 / 被重定向到登录页：** 管理员 token 不对，或会话 cookie 过期。重新登录。
- **页面能开但会话失败：** 看浏览器控制台的 WebSocket 错误。不转发 `Upgrade`/`Connection`
  的代理会破坏 `/ws`。
- **剪贴板不能用：** 大多数浏览器要求 HTTPS 才能访问剪贴板。给 hub 配 TLS。
- **hub 不可达：** 检查 `--listen` 和端口映射。Docker 在 Linux 上建议 host 网络；bridge
  网络要发布 `8787`。

## 反向代理

整个 hub 是一个 HTTP 端口，一个反向代理就够 —— 但它必须升级 WebSocket，而且不能缓冲流。

- Caddy 的 `reverse_proxy` 自动处理。
- Nginx 需要 `proxy_http_version 1.1`、`Upgrade` 和 `Connection "upgrade"`。
- 不要在 `/ws` 或 `/mcp` 前面放缓冲的 body limit。
- TLS 在代理处终止时，hub 可以继续在 loopback 上跑明文 HTTP。

见 [TLS 与反向代理](/zh/docs/install/#tls-and-the-reverse-proxy)。

## 黑屏或冻屏

- **macOS：** 画面冻住通常是显示器睡眠或 `loginwindow` 会话。确认设备使用 **macOS 账户**
  登录，然后唤醒 Mac，或用 `caffeinate -d -s` 保持唤醒。见 [macOS](/zh/docs/devices/macos/)。
- **Linux：** 服务可能绑在错误的显示上。x11vnc 的 `-display :0` 必须匹配会话；无头机器
  需要 X server 或虚拟显示。
- **Wayland：** 合成器可能没有实现远程输入；试试它自己的 VNC 后端。
- **iOS：** 画面冻住说明桥会话结束了。它不会被显示成在线；重启桥，等 15–30 秒拿 keyframe。
  见 [iOS](/zh/docs/devices/ios/)。

## agent 连不上

- 用 `list_devices` 返回的完整 `ws_url`；设备 id 区分大小写。
- 把管理员 token 放在 `Authorization: Bearer`，或者客户端不能设 header 时用 `?token=`。
  优先 header。
- `l4` 模式下客户端必须回答设备自己的 VNC 密码；`gateway` 模式下不要期待密码挑战。
- 如果客户端只会 TCP，先用 `websocat` 桥接 WebSocket。见
  [WebSocket 与 VNC](/zh/docs/agents/websocket/)。

## Docker 和 TLS

- 镜像健康检查调用明文 HTTP `/healthz`。用 `--tls-cert`/`--tls-key` 时加 `--no-healthcheck`。
- macOS 或 Windows Docker Desktop 的 host 网络有限；用 bridge 网络并发布 8787。
- 镜像以 uid 10001 运行。bind mount 的配置目录必须对这个 uid 可写：
  `chown -R 10001 ./config`。

## 重置但保留设备

停掉 hub，编辑 `config.yaml`，重启。要轮换管理员 token，改 `admin_token`（或设置
`ABACAD_ADMIN_TOKEN`）并重启；所有浏览器会话、MCP 客户端和保存的 skill 都会失效。只想要
新的浏览器会话、保留设备，就只改 `cookie_secret`。

如果 token 丢了，从文件里读：

```bash
# 原生
sed -n 's/^admin_token: //p' ~/.config/abacad/config.yaml

# Docker
docker exec abacad cat /config/config.yaml
```
