---
title: iOS 与 iPadOS
description: iPhone 和 iPad 没有 VNC 服务。iOS VNC Bridge 是一个 macOS 菜单栏应用，用一台已配对的 Mac 把设备变成标准 RFB 端点，交给 abacad 使用。不需要越狱。
---

iPhone 和 iPad **没有 VNC 服务**，不越狱也无法运行一个。hub 仍然可以列出一台，因为一台
已配对的 Mac 可以替它桥接：[iOS VNC Bridge](https://github.com/abacad-ai/ios-vnc-bridge)
是一个菜单栏应用，用 Apple 的开发者服务把设备变成标准 RFB 端点，hub 把它当成普通 VNC 服务。

```text
iPhone / iPad ──USB──▶ 运行 iOS VNC Bridge 的 Mac ──RFB :5970──▶ abacad ──▶ 浏览器
```

## Mac 需要什么

- macOS 26+，**Xcode 27** 安装并打开过一次（CoreDevice 开发者服务）。
- 设备 **已配对并信任**，**开发者模式** 已打开，并重启过一次。
- 启动会话时设备 **解锁**。
- Python 3.9+ —— Xcode 自带的就够；`pymobiledevice3` 由应用自己安装。

## 启动

从[应用 releases](https://github.com/abacad-ai/ios-vnc-bridge/releases) 下载 DMG，打开后把
**iOS VNC Bridge** 拖进 Applications，然后启动（首次需要右键 → 打开；构建是 ad-hoc 签名的）。
它住在菜单栏里。

1. **Install device runtime** —— 一次性联网下载，装进
   `~/Library/Application Support/ios-vnc-bridge`。
2. 选设备，保持 **USB**，按 **Start**。第一帧需要 **15–30 秒**（tunnel、stream、keyframe）。
   打开 **Start when a device is found** 后，设备一出现应用就开始服务。
3. 复制地址和密码，然后加进 hub。

密码在首次启动时生成，保存在 Application Support 下一个 `0600` 文件里，且始终必需。应用通过
管道把它交给 engine，所以不会出现在 `ps` 里。

## 添加到 hub

点 **添加设备**，填：

- **名称：** `iphone` 或 `ipad`
- **Host：** Mac 的局域网地址，应用里会显示
- **端口：** `5970`
- **登录方式：** **VNC 密码**
- **密码：** 应用里显示的那个
- **平台：** iOS 或 iPadOS
- **模式：** `gateway`

保存后选中设备。第一次认证成功会把它标成 **在线**。

## 用 Wi-Fi 代替 USB

USB 是默认：它是最不会飘的连接，而且配对总需要线。要走网络，先在插线状态下执行一次：

```bash
pymobiledevice3 lockdown wifi-connections on
```

然后把应用里的连接切到 **Wi-Fi** 再启动。在 iPhone 15 Pro、同一 Wi-Fi 下实测：持续运动约
34 fps，滑动到第一帧更新的延迟和 USB 一样约 105 ms。长时间静止后的第一次触摸在两种连接上
都更慢，因为设备要发新的 keyframe。

## 今天能做什么

| | |
| --- | --- |
| 帧率 | 画面变化时 25–40 fps |
| 帧大小 | 约 700 px 宽，每帧几十 KB |
| 输入 | 点击、拖动、滑动。**还不能键盘输入。** |
| 保持唤醒 | 设备上的 `PreventUserIdleSystemSleep` 断言，每 2 分钟续一次 |
| 恢复 | tunnel 断开后自动重建会话；期间断开查看者，而不是给一张冻住的画面 |

## 排错

- **列表里没有设备。** 解锁设备、点 **Trust**，再按刷新。只在 Wi-Fi 上的设备，需要先开过
  一次“通过网络连接”才会出现。
- **一按 Start 就失败。** 应用里的日志会指出卡在哪一步——多半是开发者模式没开，或者这台
  Mac 从没打开过 Xcode。
- **第一帧一直不来。** 等 30 秒；首次连接的 tunnel 和 keyframe 很慢。之后静止画面几乎不产生
  数据，所以安静的桥看起来就是安静的——日志里每两秒一行心跳才算活着。
- **“Device session ended”。** 线被拔了、设备锁了，或者 tunnel 断了。应用会自动重建会话，
  有新的帧之后查看者重连即可。

## 限制

- **不能打字。** 点击和拖动可以，输入文字不行。绕开它，或者在任务允许时用设备自己的屏幕键盘。
- **没有捏合 / 多点触控。**
- **还没处理旋转** —— 想在哪个方向看，就在启动前把设备摆成那个方向。iPad 横屏正在修。
- **受保护的视频是黑的**，任何录屏都一样。

## 安全

桥的端口就是操作手机的入口。应用始终设置密码、把它放在仅本人可读的文件里，并绑定到局域网。
在 Mac 防火墙上把 TCP 5970 限制为只允许 hub 的 IP，并把这台 Mac 当成特权主机：开发者模式正是
它能注入输入的原因。hub 会像其他设备凭据一样把这个密码保存在 `config.yaml`。见
[安全](/zh/docs/operations/security/)。
