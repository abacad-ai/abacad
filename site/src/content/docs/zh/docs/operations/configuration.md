---
title: 配置
description: abacad 的 config.yaml 字段、环境变量、命令行参数、模式和认证类型。
---

hub 的全部状态都在一个 YAML 文件里。没有数据库。网页控制台改的是同一个文件，所以两种
方式都能管理设备。

## 文件在哪

- 默认：`~/.config/abacad/config.yaml`
- `--config PATH` 或 `ABACAD_CONFIG` 覆盖。
- Docker：挂载卷里的 `/config/config.yaml`。

第一次运行会创建文件，并生成 `admin_token` 和 `cookie_secret`。权限是 `0600`，因为里面
有设备密码。

## 完整示例

```yaml
listen: 0.0.0.0:8787
admin_token: 3f2c...        # 64 个十六进制字符，自动生成
cookie_secret: 9a71...      # 给浏览器会话 cookie 签名
devices:
  - id: mac-mini
    name: Mac mini
    host: 192.168.1.92
    port: 5900
    auth: ard                # ard（macOS 账户）或 vnc
    username: iloahz         # ard 必填
    password: "账户密码"
    platform: macos
    mode: gateway            # gateway、l4 或 direct
    created_at: 2026-11-25T09:00:00Z

  - id: linux-box
    name: linux-box
    host: 192.168.1.30
    port: 5900
    auth: vnc
    password: "VNC 密码"
    platform: linux
    mode: gateway
```

## 字段

| 字段 | 含义 |
| --- | --- |
| `listen` | hub 监听地址。默认 `0.0.0.0:8787`。 |
| `admin_token` | 唯一能打开 hub 的密钥。至少 16 个字符；默认生成 64 个十六进制字符。 |
| `cookie_secret` | 给 HttpOnly 浏览器会话 cookie 签名。换掉会让所有浏览器退出。 |
| `devices[].id` | `/ws/vnc/<id>` 的路径段。从名称生成；必须唯一。 |
| `devices[].name` | 设备栏里的显示名。 |
| `devices[].host` | **hub** 访问设备用的地址：局域网 IP、VPN 地址、DNS 名，或转发到本地的端口。 |
| `devices[].port` | 设备上的 VNC 端口，通常 5900（TigerVNC 的 `:2` 显示是 5902，iOS VNC Bridge 是 5970）。hub 出站去连它；hub 从不发布它。 |
| `devices[].auth` | `vnc` 用 VNC 密码；`ard` 用 macOS 账户（RFB type 30）。 |
| `devices[].username` | macOS 短名。`auth: ard` 时必填。 |
| `devices[].password` | VNC 密码或账户密码。`l4` 模式下如果不想保存可以不填。 |
| `devices[].platform` | 自由标签，用于图标和状态提示：`macos`、`windows`、`linux`、`android`、`ios`。 |
| `devices[].mode` | `gateway`（默认）、`l4` 或 `direct`。 |
| `devices[].created_at` | 添加设备时写入，仅供参考。 |

### 模式

| 模式 | hub 做什么 | 客户端认证 |
| --- | --- | --- |
| `gateway` | 终止 RFB 握手，替设备登录，然后传字节。 | 无；hub 处理。 |
| `l4` | 通过 WebSocket 做盲 TCP 转发。 | 客户端回答设备自己的 VNC 密码。 |
| `direct` | 只做目录，没有 WebSocket。 | 客户端自己连 `host:port`。 |

模式改变的是 WebSocket 里面怎么处理，不是你要发布什么：不管哪种模式，hub 都只监听那一个
HTTP 端口。

### 认证类型

| auth | RFB 安全类型 | 用于 |
| --- | --- | --- |
| `vnc` | Type 2（经典 VNC 密码） | Windows、Linux、Android、iOS VNC Bridge、旧版 macOS。 |
| `ard` | Type 30（Apple DH） | 使用真实账户的 macOS，附着到 console 会话。 |

## 环境变量和参数

| 变量 | 参数 | 含义 |
| --- | --- | --- |
| `ABACAD_CONFIG` | `--config PATH` | 配置文件路径。 |
| `ABACAD_LISTEN` | `--listen ADDR` | 监听地址。 |
| `ABACAD_ADMIN_TOKEN` | 无 | 管理员 token。只在运行时生效，不会写进文件。至少 16 个字符。 |
| 无 | `--tls-cert FILE` | 用这个证书提供 HTTPS/WSS。 |
| 无 | `--tls-key FILE` | 证书私钥。 |
| 无 | `-v` | 详细日志。 |

flag 优先于环境变量，环境变量优先于文件。`abacad` 不带参数就是 serve；`serve` 是为显式
compose 文件保留的。`abacad version` 打印构建版本。

## 编辑与轮换

- **手改的文件在启动时读取。** 运行中的 hub 会在 UI 里增删改设备时写回文件。自己改完请
  重启。
- **轮换 `admin_token`** 会立即让所有浏览器会话、agent 连接和保存的 skill 失效。更新 MCP
  客户端里的 token，重新生成 `SKILL.md`。
- **轮换 `cookie_secret`** 会让所有浏览器退出，但不影响管理员 token 或 MCP。
- **备份文件。** 里面有 token 和所有设备密码。没有它你可以重建设备列表，但找不回密钥。

## 健康和日志

镜像的健康检查是监听端口上的 `GET /healthz`。用 `--tls-cert`/`--tls-key` 的 TLS 部署应该
给 Docker 加 `--no-healthcheck`，因为这个探测是明文 HTTP。排查连接时用 `-v`；hub 会记录
设备、模式和握手结果。
