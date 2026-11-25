---
title: 安装
description: 用 Docker 或单个 Go 二进制运行 abacad。配置 TLS、反向代理、配置文件、升级和健康检查。
---

hub 是一个内嵌前端的二进制。Node 只是构建依赖；运行中的 hub 不需要 Node、数据库或 CDN。

## Docker

每个 tagged release 都会为 **linux/amd64** 和 **linux/arm64** 发布预构建镜像：

```bash
docker run -d --name abacad --network host \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest

docker logs abacad
```

然后打开 `http://<host>:8787`。

Linux 上强烈建议 `--network host`：这样 hub 日志里能看到真实的客户端 IP，防火墙规则也
更容易判断。仓库里的 compose 文件就是同一套配置：

```bash
docker compose up -d
```

### 环境变量

镜像把状态放在卷里的 `/config`，部署设置从环境变量读取。命令行 flag 的优先级高于两者。

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `ABACAD_CONFIG` | `/config/config.yaml` | 容器里的配置文件路径 |
| `ABACAD_LISTEN` | `0.0.0.0:8787` | 监听地址；健康检查跟随它的端口 |
| `ABACAD_ADMIN_TOKEN` | 自动生成 | 管理员 token，至少 16 个字符，不会写进 `config.yaml` |

生成的 token 会打印在 `docker logs` 里。想从密钥管理器固定一个：

```bash
docker run -d --name abacad --network host \
  -e ABACAD_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest
```

设备保存在卷里的 `config.yaml` 中：`docker compose down` 会保留，`docker compose down -v`
会删除。想把文件放在宿主机上，就 bind mount `./config:/config` 并
`chown -R 10001 ./config`（镜像以 uid 10001 运行）。

<a id="bridge-networking-instead-of-host"></a>

### 用 bridge 网络代替 host

如果不能用 host 网络 —— macOS 或 Windows Docker Desktop，或者宿主机命名空间不归你管 ——
就发布 8787。这一个端口承载控制台、API、MCP 和所有设备 WebSocket：

```bash
docker run -d --name abacad \
  -p 8787:8787 \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest
```

compose 的等价写法见仓库里的 `compose.bridge.yaml`。

### 健康检查

镜像每 30 秒调用一次 `ABACAD_LISTEN` 端口上的 `GET /healthz`。这个探测是明文 HTTP，所以
用 `--tls-cert`/`--tls-key` 做 TLS 部署时应该加 `--no-healthcheck`。

### 升级

```bash
docker compose pull && docker compose up -d
docker run --rm ghcr.io/abacad-ai/abacad:latest version
```

`/config` 卷不会被改动，设备会保留。tag 有 `latest`、`3`、`3.0`、`3.0.0`；想按自己的节奏
升级就固定一个版本。

3.0 去掉了裸 VNC 端口和每设备 token：所有设备都走 hub 的 WebSocket，管理员 token 是唯一
的密钥。旧的配置文件仍然能加载；被去掉的字段会在下一次保存时消失。

<a id="the-binary"></a>

## 二进制

要求 Go 1.27+；只有从源码构建前端时才需要 Node 24。

```bash
git clone https://github.com/abacad-ai/abacad.git
cd abacad
make build          # npm ci + vite build + go build
./bin/abacad serve
```

或者分开构建：

```bash
cd frontend && npm ci && npm run build && cd ..
go build -o abacad ./cmd/abacad
./abacad serve
```

第一次运行会创建 `~/.config/abacad/config.yaml`，并打印 URL 和管理员 token。

```text
abacad [serve] [--config PATH] [--listen ADDR]
               [--tls-cert FILE --tls-key FILE] [-v]
abacad version
```

不带参数就是 serve。`serve` 是为显式 compose 文件保留的。`ABACAD_CONFIG`、
`ABACAD_LISTEN`、`ABACAD_ADMIN_TOKEN` 可以从环境变量设置同样的东西；flag 优先。

<a id="tls-and-the-reverse-proxy"></a>

## TLS 与反向代理

用 HTTPS/WSS。大多数浏览器的剪贴板需要安全上下文，管理员 token 也不该在你无法控制的
明文网络里传输。

二进制可以自己终止 TLS：

```bash
./abacad serve --tls-cert cert.pem --tls-key key.pem
```

用浏览器信任的证书。也可以让反向代理终止 TLS。整个 hub 只有一个 HTTP 端口，所以一个
代理就能覆盖控制台、API、MCP、noVNC 和每台设备的 WebSocket。

Caddy 会自动处理 WebSocket 升级：

```text
abacad.example.com {
  reverse_proxy 127.0.0.1:8787
}
```

Nginx 需要 upgrade 头：

```nginx
server {
  listen 443 ssl;
  server_name abacad.example.com;

  location / {
    proxy_pass http://127.0.0.1:8787;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }
}
```

如果 TLS 在代理处终止，让 hub 继续跑明文 HTTP，并绑定到 loopback 或私有网卡。不要把
8787 直接发布到公网。

## 配置文件

和网页控制台写的是同一个文件。YAML，权限 `0600`，设备密码就在里面。

```yaml
listen: 0.0.0.0:8787
admin_token: 3f2c...
cookie_secret: 9a71...
devices:
  - id: mac-mini
    name: Mac mini
    host: 192.168.1.92
    port: 5900
    auth: ard              # ard = macOS 账户，vnc = VNC 密码
    username: iloahz
    password: "账户密码"
    platform: macos
    mode: gateway          # gateway、l4 或 direct
```

- `id` 是在 UI 里添加设备时从名称生成的。它是 `/ws/vnc/<id>` 的路径段；如果 agent 已经
  保存了这个 URL，就保持它不变。
- `cookie_secret` 给浏览器会话 cookie 签名。换掉它会让所有浏览器退出登录。
- `admin_token` 是唯一能打开 hub 的密钥。改文件（或 `ABACAD_ADMIN_TOKEN`）后重启即可轮换。
- 生成的 token 是 64 个十六进制字符。手写的至少 16 个字符。
- 手改的文件在启动时读取。运行中的 hub 会在 UI 里改设备时写回文件；自己改完请重启。

每个字段和模式见 [配置](/zh/docs/operations/configuration/)。
