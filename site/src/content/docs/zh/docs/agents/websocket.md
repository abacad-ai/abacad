---
title: WebSocket 与 VNC
description: 每台设备都是 /ws/vnc/<id> 上的 websockify 风格 WebSocket，里面是裸 RFB。浏览器、agent，或者经 websocat 桥接的任意 VNC 客户端都能用。
---

hub 进入设备的唯一方式：每台设备在 **`/ws/vnc/<id>`** 上有一个 WebSocket，里面是裸 RFB。
这是 noVNC 使用的 websockify 约定。字节里没有任何 abacad 专有的东西，所以任何能连
WebSocket（或本地 TCP 端口）的 VNC 客户端都能用。

## 端点

```text
ws://<hub>/ws/vnc/<device id>
wss://<hub>/ws/vnc/<device id>
```

设备 id 就是设备栏和 `list_devices` 里的那个。用管理员 token 认证：

```text
Authorization: Bearer <admin token>
```

不能设置 header 的客户端可以改用查询参数：

```text
wss://<hub>/ws/vnc/mac-mini?token=<admin token>
```

优先用 header。URL 里的 token 会出现在代理日志、shell 历史和浏览器历史里。

## 客户端看到什么

握手取决于设备的模式：

| 模式 | 客户端的 RFB 握手 |
| --- | --- |
| `gateway`（默认） | hub 在上游替客户端登录设备，对客户端呈现 **None** 握手。客户端看不到 VNC 密码。 |
| `l4` | hub 是盲管。客户端回答设备自己的 VNC 密码。 |
| `direct` | 不提供 WebSocket。客户端直连设备。 |

`gateway` 模式下，hub 只终止 RFB 握手，然后双向复制字节。它不解码 framebuffer，也不合成
输入。

## 桥到本地端口

只会 TCP 的客户端用 `websocat`：

```bash
websocat --binary \
  -H="Authorization: Bearer <admin token>" \
  tcp-l:127.0.0.1:5900 \
  ws://192.168.1.10:8787/ws/vnc/mac-mini
```

然后把任何 VNC 客户端指向 `127.0.0.1:5900`。如果设备是 `l4` 模式，客户端会问设备的 VNC
密码；`gateway` 模式下不会问。

## 在浏览器里

控制台把内置的 noVNC 应用放在 iframe 里。同一个查看器也可以直接打开：

```text
http://<hub>/vnc/<device id>
```

这个路由需要一个已登录的浏览器会话。WebSocket 本身接受管理员 token，agent 用的就是它。

## 注意

- **一个 WebSocket 就是一个会话。** 每个查看者开一个。有些 VNC 服务允许多个查看者，有些不允。
- **不录屏、不 OCR、不合成输入。** hub 是一根管子。需要截图或点击，由 VNC 客户端做。
- **在线状态不是这个连接。** 一次认证成功的会话把设备标成在线十分钟；hub 不会探测来刷新它。
- **TLS 很重要。** `https` 的 hub 对应 `wss`，管理员 token 不会明文过网。见
  [TLS 与反向代理](/zh/docs/install/#tls-and-the-reverse-proxy)。
