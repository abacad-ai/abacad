---
title: abacad 是什么
description: abacad 是一个小巧的自托管 VNC hub。它把机器上已有的 VNC 服务接到一个浏览器控制台，并为 agent 提供每台设备一个 WebSocket。
---

abacad 让机器上已经在跑的 VNC 服务有 **一个统一的家**。你在浏览器里打开任意一台；
agent 用同一个管理员 token 打开同一个画面。设备上不用装任何 abacad 软件，中间没有云端，
也没有中继。

```text
  macOS 屏幕共享 ───────┐
  Linux x11vnc/wayvnc ──┤
  Windows TigerVNC ─────┼── RFB ──▶  abacad hub  ── HTTPS/WSS ──▶ 浏览器 (noVNC)
  Android droidVNC-NG ──┤              │
  iOS/iPadOS 经 Mac 应用 ─┘            └── 同一个 WSS ──▶ agent 的 VNC 客户端
```

<dl class="glance">
  <div><dt>设备上</dt><dd>不装 abacad</dd></div>
  <div><dt>hub 到设备</dt><dd>标准 RFB</dd></div>
  <div><dt>到客户端</dt><dd>每台设备一个 WebSocket</dd></div>
  <div><dt>要管理</dt><dd>一个端口，一个 token</dd></div>
</dl>

## 一个画面，两种客户端

浏览器和 agent 不是两套集成，而是同一个 WebSocket 的两个客户端。地址是
`/ws/vnc/<id>`，里面是裸 RFB —— 也就是 noVNC 已经在用的 websockify 约定。浏览器拿到的是
内置的 noVNC 应用；agent 拿到字节，用任何它喜欢的 VNC 客户端。

这就是产品的全部。abacad 不是一套新的设备 API，也不是第二种屏幕协议。它只是让你和
agent 都能到达你本来就已经信任的 VNC 服务的最薄一层。

每个查看者都是独立的 RFB 会话。人和 agent 能否同时连上同一台设备，取决于 VNC 服务是否
允许多个查看者。`x11vnc` 和 TigerVNC 加 `-shared` 可以；有些服务不行。如果不行，就关掉
agent 的会话，在浏览器里完成这一步，再让 agent 重连。设备状态是一样的。

## 它不是什么

- **不是托管服务。** hub 由你运行，网络路径由你掌控。你和设备之间没有账号、没有中继，
  也没有厂商。
- **不是远程桌面的重新实现。** 画面属于 VNC 服务；abacad 不解码 framebuffer、不录屏、
  也不合成输入。
- **不是设备发现服务。** 没有 mDNS、没有扫描、没有广播列表。只有你填了 host 和 port，
  一台机器才存在于 abacad 里。
- **不是审批层。** `gateway` 模式下 hub 替客户端登录设备，但不会要求你逐步批准每个动作。
  如果你需要审批，请在 agent 外面自己加策略。

## 模式

每台设备有一个模式，默认是 `gateway`。

| 模式 | 浏览器和 agent | 凭据处理 |
| --- | --- | --- |
| `gateway`（默认） | 可以，走 hub 的 WebSocket | hub 保存设备密码，客户端看不到 VNC 密码 |
| `l4` | 可以，客户端回答设备自己的 VNC 密码 | hub 是盲管，不需要保存密码 |
| `direct` | 不行，只做目录 | agent 直连设备；hub 不承载 VNC 流量 |

`gateway` 模式下，hub 只终止 RFB 握手。之后它就是一根盲管。在 macOS 上，它执行 Apple
的账户登录（RFB type 30），这样会话会附着到已登录用户的 console，而不是 `loginwindow`。

<a id="online"></a>

## 在线状态

abacad **永远不会主动连接设备**。没有定时检查、没有健康检查命令，也没有 **检查** 按钮。
只有真实的浏览器或 agent 会话认证成功，设备才会被标记为 **在线**；这个状态十分钟后会
过期。你从未打开过的设备显示 **未见过**。

这是故意的。TigerVNC 把任何在认证前关闭的连接都算作安全失败，而且计数器不会衰减：
即使是一个行为良好的探测，也可能最终把 hub 拉黑。abacad 唯一的连接，就是你和 agent
明确要求的那一次。如果你接手了一个黑名单，见 [排错](/zh/docs/operations/troubleshooting/)。

## 平台

每篇平台指南都覆盖 VNC 服务、凭据、防火墙，以及最容易卡住的地方。下面这五块屏幕都来自
同一个 hub、同一个浏览器控制台：

<div class="platform-gallery">
  <a class="platform-gallery-item" href="/zh/docs/devices/macos/">
    <img src="/screens/live-macos-thumb.webp" alt="通过 VNC 截取的 Mac mini 桌面" loading="lazy" />
    <span>macOS</span>
  </a>
  <a class="platform-gallery-item" href="/zh/docs/devices/windows/">
    <img src="/screens/live-windows-thumb.webp" alt="通过 VNC 截取的 Windows 11 桌面" loading="lazy" />
    <span>Windows</span>
  </a>
  <a class="platform-gallery-item" href="/zh/docs/devices/linux/">
    <img src="/screens/live-linux-thumb.webp" alt="通过 VNC 截取的 Debian 锁屏" loading="lazy" />
    <span>Linux</span>
  </a>
  <a class="platform-gallery-item" href="/zh/docs/devices/android/">
    <img src="/screens/live-android-thumb.webp" alt="通过 VNC 截取的 Android 桌面" loading="lazy" />
    <span>Android</span>
  </a>
  <a class="platform-gallery-item" href="/zh/docs/devices/ios/">
    <img src="/screens/live-iphone-thumb.webp" alt="通过 VNC 截取的 iPhone 桌面" loading="lazy" />
    <span>iOS</span>
  </a>
</div>

<div class="platform-cards">
  <a class="platform-card" href="/zh/docs/devices/macos/">
    <strong>macOS</strong>
    <span>屏幕共享 + macOS 账户，附着到 console 会话</span>
  </a>
  <a class="platform-card" href="/zh/docs/devices/windows/">
    <strong>Windows</strong>
    <span>TigerVNC 服务模式，能覆盖登录界面</span>
  </a>
  <a class="platform-card" href="/zh/docs/devices/linux/">
    <strong>Linux</strong>
    <span>x11vnc、TigerVNC、wayvnc，以及 Wayland 的替代方案</span>
  </a>
  <a class="platform-card" href="/zh/docs/devices/android/">
    <strong>Android</strong>
    <span>droidVNC-NG，以及 MediaProjection 的注意事项</span>
  </a>
  <a class="platform-card" href="/zh/docs/devices/ios/">
    <strong>iOS 与 iPadOS</strong>
    <span>由已配对的 Mac 运行 iOS VNC Bridge，不需要越狱</span>
  </a>
</div>

## 下一步

- [快速开始](/zh/docs/getting-started/) — 跑起 hub，接进第一台机器。
- [安装](/zh/docs/install/) — Docker、二进制、TLS 和反向代理。
- [MCP](/zh/docs/agents/mcp/) — 让 agent 连上 `POST /mcp`。
- [安全](/zh/docs/operations/security/) — 威胁模型和网络边界。
