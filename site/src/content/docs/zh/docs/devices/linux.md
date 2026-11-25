---
title: Linux
description: 为 abacad 配置 x11vnc、TigerVNC 或 Wayland 的 VNC 服务，附 systemd 单元、防火墙规则和 blacklist 注意事项。
---

按你的会话类型选 VNC 服务。X11 和 Wayland 是两个不同的问题；`x11vnc` 在 Wayland 会话上
不能工作。

## X11：x11vnc

```bash
sudo apt install x11vnc
x11vnc -storepasswd                 # 写入 ~/.vnc/passwd
x11vnc -display :0 -rfbauth ~/.vnc/passwd \
  -forever -shared -rfbport 5900 -listen 0.0.0.0
```

一个 systemd 用户单元：

```ini
# ~/.config/systemd/user/x11vnc.service
[Unit]
Description=x11vnc for abacad
After=graphical-session.target

[Service]
ExecStart=/usr/bin/x11vnc -display :0 -rfbauth %h/.vnc/passwd -forever -shared -rfbport 5900 -listen 0.0.0.0
Restart=on-failure

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable --now x11vnc.service
```

`-shared` 允许第二个查看者；不加的话，第二个浏览器或 agent 连接可能被拒绝。如果只想
一次一个客户端，可以不加。

## X11：TigerVNC

TigerVNC 的用户配置在 `~/.config/tigervnc/config`：

```ini
localhost = no
SecurityTypes = VncAuth
geometry = 1600x900
depth = 24

# 可选加固：提高源 IP 拉黑的门槛。这不会清除已有的拉黑；改完要重启服务。
BlacklistThreshold = 1000000
```

作为系统服务运行：

```bash
sudo systemctl enable --now tigervncserver@:2.service   # RFB 端口 5902
```

TigerVNC 把每个在认证前关闭的连接都算作安全失败。计数器不衰减，所以任何工具反复探测
最终都会拉黑那个 IP，连正确的客户端也会收到 `Too many security failures`。不要用
`nc`、负载均衡、监控或健康检查去探测端口。

abacad 自己不会探测：只有你打开浏览器会话，或 agent 打开设备 WebSocket 时，它才连接。
清除已有的拉黑，重启 VNC 服务：

```bash
sudo systemctl restart tigervncserver@:2.service
```

## Wayland

- **wlroots（sway 等）**：`wayvnc 0.0.0.0 5900`，配置在 `~/.config/wayvnc/config`。
- **KDE Plasma**：`krfb`，在设置里配端口和密码。
- **GNOME**：`gnome-remote-desktop` 有 VNC 后端：

  ```bash
  grdctl vnc set-auth-method password
  grdctl vnc set-password
  grdctl vnc disable-view-only
  grdctl vnc enable
  systemctl --user enable --now gnome-remote-desktop.service
  ```

  不同发行版对 VNC 后端的打包或启用方式不同；用 `grdctl vnc --help` 确认。

Wayland 合成器的输入支持不一样。`wayvnc` 在 wlroots 合成器上支持指针和键盘输入；`krfb`
和 `gnome-remote-desktop` 各有自己的限制。在把机器交给 agent 之前，先实际点一下、打几个字。

## 防火墙

只允许 hub 访问 VNC 端口：

```bash
sudo ufw allow from 192.168.1.10 to any port 5900 proto tcp
```

用 hub 的 IP。不要把 5900 开到公网。

## 添加到 hub

在控制台点 **添加设备**：

- **名称：** `linux-box`
- **Host：** 机器 IP，例如 `192.168.1.30`
- **端口：** `5900`（TigerVNC 的 `:2` 显示是 `5902`）
- **登录方式：** **VNC 密码**
- **密码：** 上面配置的密码
- **平台：** Linux
- **模式：** `gateway`

没有健康检查要配：abacad 从不探测端口。保存后选中一次机器；会话认证成功时设备栏显示
**在线**，在那之前一直是 **未见过**。

## 排错

- **连接被拒绝。** 检查服务监听在 hub 能到达的网卡上：`ss -ltnp | grep 5900`。
  `-listen 127.0.0.1` 只能本机访问。
- **连上后黑屏。** 服务可能绑到了错误的显示。x11vnc 检查 `-display :0` 是否匹配会话；
  无头机器需要 X server 或虚拟显示。
- **输入没反应。** Wayland 合成器可能没有实现远程输入。试试合成器自己的 VNC 后端，或
  换成 X11 会话。
- **`Too many security failures`。** 重启 VNC 服务；黑名单在内存里，不会自己衰减。

在把 hub 暴露到局域网之外以前，先读 [安全](/zh/docs/operations/security/)。
