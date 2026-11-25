---
title: Windows
description: 在 Windows 上用 TigerVNC 服务模式，让 abacad 能到登录界面和桌面，并把防火墙规则限制到 hub。
---

用 **TigerVNC** 的服务模式。服务模式能显示 Windows 登录界面，用户会话模式的服务器做不到。

## 安装

1. 从 <https://tigervnc.org/> 下载 TigerVNC。
2. 安装时勾选 **Register TigerVNC as a system service**。
3. 按提示设置 VNC 密码（或运行 `tvnserver` → **Configure** → **Security**）。
4. 服务监听 TCP **5900**。

TigerVNC 密码受 RFB 安全类型限制，只有八个字符有效。选一个强的八位密码；保护端口的是
网络边界，不是密码长度。

## 防火墙

创建入站规则，只允许 hub 的 IP：

```powershell
New-NetFirewallRule -DisplayName "abacad VNC" -Direction Inbound `
  -Protocol TCP -LocalPort 5900 -RemoteAddress 192.168.1.10 -Action Allow
```

不要创建 any-source 规则；不要在路由器上转发 5900。

## 添加到 hub

在控制台点 **添加设备**：

- **名称：** `windows-pc`
- **Host：** 机器 IP，例如 `192.168.1.40`
- **端口：** `5900`
- **登录方式：** **VNC 密码**
- **密码：** TigerVNC 密码
- **平台：** Windows
- **模式：** `gateway`

保存后选中机器。第一次成功会话会把它标成 **在线**。

## 排错

- **打开后一直离线。** 检查入站防火墙规则，以及 TigerVNC 是否作为服务运行，而不只是
  托盘程序。
- **登录界面之后黑屏。** 服务可能跑在非 console 的会话里。重启 TigerVNC 服务再重连。
- **`Too many security failures`。** TigerVNC 会在反复失败后拉黑 IP，计数器不衰减。重启
  服务清除，然后停掉任何探测端口的工具。见 [排错](/zh/docs/operations/troubleshooting/)。
- **键盘布局不对。** noVNC 发送的是 keysym；在 noVNC 控制栏把键盘布局改成和 Windows
  一致。

在把 hub 暴露到局域网之外以前，先读 [安全](/zh/docs/operations/security/)。
