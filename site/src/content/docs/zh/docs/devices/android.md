---
title: Android
description: 用 droidVNC-NG 作为 Android 手机或平板的 VNC 服务，处理 MediaProjection 授权，并添加进 abacad。
---

用 **droidVNC-NG**（F-Droid 或 Google Play）。它是标准的 VNC 服务，自带一个可选的 noVNC
页面；abacad 不需要那个页面，只需要 RFB 端口。

## 配置

1. 安装 droidVNC-NG。
2. 设置密码和端口（默认 **5900**）。
3. 如果要远程输入，启用 **AccessibilityService**；不启用的话只有画面。
4. 点 **Start**。
5. 保持设备能被 hub 访问到。

在控制台点 **添加设备**，填：

- **名称：** `pixel`
- **Host：** 手机 IP，例如 `192.168.1.50`
- **端口：** `5900`（或你设置的端口）
- **登录方式：** **VNC 密码**
- **密码：** droidVNC-NG 的密码
- **平台：** Android
- **模式：** `gateway`

## Android 10+ 的屏幕捕获

Android 10 及更高版本每次启动都需要 MediaProjection 授权弹窗。选项：

- 每次重启后接受弹窗。
- 用 adb 授权一次（需要 device owner 或调试）：

  ```bash
  adb shell cmd appops set net.christianbeier.droidvnc_ng PROJECT_MEDIA allow
  ```

- 如果你的版本支持，用 app 的 intent 接口以 fallback 捕获模式启动。

这个授权是 Android 平台的规则，不是 droidVNC-NG 的限制。重启后仍然可能需要点一下。

## 电源与可靠性

- 关掉 droidVNC-NG 的电池优化。
- 如果需要重启后自动运行，打开 **Start on Boot**。
- 保持充电；Android 可能因为电量策略杀掉后台服务。
- Android 16+ 上，一些 app 组件可以选择不接受 accessibility 输入；那些界面不会响应
  远程点击。

## 添加到 hub

保存后选中手机一次。第一次认证成功会把它标成 **在线**。如果一直是 **未见过**，检查 hub
能否访问手机 IP，以及 droidVNC-NG 服务是否在运行（锁屏或省电模式可能会停掉它）。

## 关于“零接触”

droidVNC-NG 是标准组件，但 Android 的 MediaProjection 规则意味着重启后可能需要点一下。
如果零接触是硬需求，这是唯一值得考虑在 app 内嵌一个小 VNC 服务的场景，而且应该作为明确
的例外，不是 hub 的核心。

在把 hub 暴露到局域网之外以前，先读 [安全](/zh/docs/operations/security/)。
