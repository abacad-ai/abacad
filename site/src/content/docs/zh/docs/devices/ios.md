---
title: iOS 与 iPadOS
description: iPhone 和 iPad 没有 VNC 服务。用一台已配对的 Mac 运行 iosvnc 桥，把设备变成标准 RFB 端点，交给 abacad 使用。不需要越狱。
---

iPhone 和 iPad **没有 VNC 服务**，不越狱也无法运行一个。hub 仍然可以列出一台，因为一台
已配对的 Mac 可以替它桥接：[`iosvnc`](https://github.com/abacad-ai/abacad/tree/main/iosvnc)
用 Apple 的开发者服务把设备变成标准 RFB 端点，hub 把它当成普通 VNC 服务。

```text
iPhone ──USB──▶ 运行 iosvnc 的 Mac ──RFB :5970──▶ abacad ──▶ 浏览器
```

## Mac 需要什么

- macOS 和 **Xcode 27**（CoreDevice 开发者服务）。
- 设备 **已配对并信任**，**开发者模式** 已打开。
- 启动桥时设备 **解锁**，并接着 **USB**。
- Python 3.9+，venv 里装 `pymobiledevice3`。
- **不需要 root**：在 macOS 上桥走进程内的 userspace tunnel。
- Go 1.27+ 来构建桥（`serve-ios.sh` 会替你构建）。

## 启动桥

在仓库的 `iosvnc/` 目录：

```bash
cd iosvnc
python3 -m venv .venv && .venv/bin/pip install pymobiledevice3
./serve-ios.sh 192.168.1.10:5970 --password Abcd1234
```

第一帧需要 **15–30 秒**（tunnel、stream、keyframe）。桥启动后会在设备上持有一个电源断言，
所以会话期间屏幕不会锁。

桥监听局域网地址时一定要传 `--password`。不传的话，任何能访问 5970 的人都能操作手机。

## 添加到 hub

点 **添加设备**，填：

- **名称：** `iphone`
- **Host：** Mac 的局域网地址，例如 `192.168.1.10`
- **端口：** `5970`
- **登录方式：** **VNC 密码**
- **密码：** 传给桥的 `--password`
- **平台：** iOS
- **模式：** `gateway`

保存后选中设备。第一次认证成功会把它标成 **在线**。

## 用 Wi-Fi 代替 USB

设备也可以走网络，而不是线缆：

```bash
pymobiledevice3 lockdown wifi-connections on   # 先在插线状态下执行一次
./serve-ios.sh 0.0.0.0:5970 --password Abcd1234 --connection wifi
```

这个开关就是标准的“通过网络连接此 iPhone”。它会让 usbmuxd 把设备列两次，每个接口一次；
`--connection wifi` 会从网络条目建立整个会话（tunnel、显示流、HID）。在 iPhone 15 Pro、
同一 Wi-Fi 下实测：持续运动约 34 fps，滑动到第一帧更新的延迟和 USB 一样约 105 ms。长时间
静止后的第一次触摸在两种连接上都更慢，因为设备要发新的 keyframe。

USB 仍然是默认：它是不会飘的连接，而且配对本身仍然需要线。

## 今天能做什么

| | |
| --- | --- |
| 帧率 | 画面变化时 25–40 fps |
| 帧大小 | 约 78 KB（Tight/JPEG，iPhone 15 Pro 上 592×1280） |
| 输入 | 点击、拖动、滑动。**还不能键盘输入。** |
| 保持唤醒 | 设备上的 `PreventUserIdleSystemSleep` 断言，每 2 分钟续一次 |
| 查看者 | 每个会话一个 |

键盘输入是明显的缺口：点击和拖动可以，打字不行。需要输入文字的任务要绕开这一点，或者在
任务允许时用设备自己的屏幕键盘。

## 排错

- **桥找不到设备。** 解锁、信任配对、打开开发者模式、重新插线。用 `iosvnc devices` 检查。
- **第一帧一直不来。** 等 30 秒；第一次连接的 tunnel 和 keyframe 很慢。还不行就重启桥，
  看日志。
- **屏幕安静后会话断开。** 静止画面不产生帧；桥每两秒打一行心跳。日志停了说明桥卡住，
  会被重启。这是故意的：死掉的会话不会显示成在线。
- **画面冻住。** 冻结的帧和静止画面看起来一样。桥是唯一事实来源；桥结束时 hub 会关闭
  查看器，而不是留一张旧图。
- **设备锁了。** 桥持有断言，但手动锁屏仍然优先。长会话请保持设备解锁并接着电源。

## 安全

桥的端口就是操作手机的入口。把 TCP 5970 限制为只允许 hub 的 IP，至少绑定到私有网卡，并且
始终设置 `--password`。hub 会像其他设备凭据一样把这个密码保存在 `config.yaml`。见
[安全](/zh/docs/operations/security/)。
