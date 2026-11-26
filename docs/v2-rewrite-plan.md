# abacad v2（薄版）：自托管 VNC 聚合网关

> 状态：计划稿，**已被 3.0 取代**。
> 2026-11-26 更新：3.0 起每台设备都走 hub 的 `/ws/vnc/<id>`，hub 只监听一个 HTTP 端口，管理员
> token 是唯一密钥。本文 D4–D6、D8、D9、D11 以及 §2.2、§2.4、§6 里「裸 VNC / 一设备一端口 /
> per-device token」的方案**已作废**，保留作历史记录。现行部署说明见 [README](../README.md) 和
> [install](../site/src/content/docs/docs/install.md)。
> 2026-11-24 更新：mDNS 与主动探测已完全移除；定位已改为「自托管、不做公网转发/中继」，hub 可连接任意可达的 VNC 端点。
> 一句话：**一个自托管的 VNC 聚合站**。设备端用各平台现成的 VNC server，人不写客户端，agent 直接说 VNC。我们只做「目录 + 入口 + 认证 + 转发」。

---

## 0. 先回答四个问题

### VNC 走什么协议？

**RFB（Remote Framebuffer），RFC 6143，TCP 5900 起（5900 + display number）。**

- 二进制、有状态、**服务器先说话**：server 发 `RFB 003.008\n`，client 回版本，然后协商安全类型、认证、`ClientInit` / `ServerInit`。
- 之后是双向消息：client 发 `FramebufferUpdateRequest` / `KeyEvent` / `PointerEvent`，server 回 `FramebufferUpdate` / `Bell` / `ServerCutText`。
- 编码有 Raw、CopyRect、Hextile、Tight、ZRLE、JPEG 等；认证有 None、VNC（8 字符 DES）、VeNCrypt/TLS、Apple DH 等。
- **它不是 HTTP**：没有 header、没有请求/响应语义、没有 URL 路由。浏览器要用它，必须在中间放一个 WebSocket ⇄ TCP 的代理（noVNC + websockify 就是干这个的）。

### 能不能转发 VNC 流量？

**能，而且有两层做法。**

| 做法 | 做什么 | 代码量 | 能力 |
|---|---|---|---|
| **L4 转发** | `hub:5901` → `device:5900`，`io.Copy` 双向 | 几十行 | 透明、简单、设备自己认证 |
| **L7 握手网关** | 替客户端完成 RFB 握手（认证/注入密码），之后仍然纯字节管道 | 约 500 行 | 统一入口、凭据不下发、可审计、可吊销 |
| **完整 RFB 客户端** | 解码 framebuffer、合成输入 | 数千行 | 能做截图/点击 API，但这就是上一版的重方案 |

关键区别：**L7 网关只碰握手，不碰画面。** 握手完成后它就是一个盲管道。所以它仍然是「薄」的，不是重写一个 VNC 客户端。

### 能不能把 VNC 转成 HTTPS？

**分两种消费者：**

- **浏览器：能，而且这是标准做法。** 页面走 HTTPS，noVNC 走 WSS，hub 把 WSS 拆成 TCP 连到设备。浏览器看到的是 `https://hub/vnc/mac-mini`，不是 `vnc://`。
- **agent / 原生 VNC 客户端：不能。** 标准 VNC 客户端不会说 HTTPS/WebSocket。你可以把 RFB 包进 TLS（VeNCrypt）或 SSH，但那是 TLS/SSH，不是 HTTPS；而且很多 VNC server 不支持 VeNCrypt。
- 所以正确的形态是**两个前门**：`WSS` 给浏览器，`TCP` 给 agent。不要试图把两者统一成 HTTPS。

### 转发能减少暴露面吗？

**能减少一部分，但不是本质性的。**

- 减少的部分：设备只对 hub 的 IP 开 5900，其他机器不能直接连；入口收敛到一个 HTTPS 端口 + 一组 VNC 端口；认证、限流、审计可以集中在 hub；吊销一个 token 就能踢掉一个 agent。
- 没减少的部分：hub 自己变成高价值目标；hub → 设备那一段仍然是 RFB（明文，除非上 TLS）；VNC 的 8 字符 DES 认证本来就不强；HTTPS 只保护浏览器那一段。
- 真正有效的还是：**VLAN/防火墙 + VNC 不直接暴露公网 + 设备只信任 hub IP**。协议转换本身不解决信任问题。

---

## 决策记录（Decision Log）

> 目的：每个决定都写清楚「选了什么、为什么、放弃了什么、以后要改必须同时判断哪些因素」。避免只留下一个没头没尾的结论，后来的人不敢改，或者一改就踩到别的决定。
>
> 原则：决定是当前约束下的最优解，不是永久真理。改决定不可怕，漏掉耦合因素才可怕。

### 总览

| ID | 决定 | 一句话理由 | 改动成本 |
|---|---|---|---|
| D1 | 自托管，不做公网转发/中继 | 公网中继成本高、更慢更暴露；网络路径交给用户 | 极高 |
| D2 | 设备协议只用 VNC/RFB | 各平台有现成 server，零自研客户端 | 极高 |
| D3 | Hub 只做聚合网关，不做 RFB 客户端 | 不解码画面/不合成输入，1.8k 行而非 5k | 高 |
| D4 | ~~两个前门：WSS 给浏览器，TCP 给 agent~~ **3.0 已作废** | 两边都说自己的标准协议 | 中 |
| D5 | ~~浏览器默认，裸 VNC 按设备 opt-in~~ **3.0 已作废** | 避免默认铺端口，降低暴露面 | 低 |
| D6 | ~~裸 VNC 一设备一端口~~ **3.0 已作废** | RFB 没有 Host/SNI，无法单端口路由 | 中 |
| D7 | L7 握手网关为核心，L4 direct 为模式 | 密码不下发、token 可吊销，但仍只碰握手 | 中 |
| D8 | AI 接口 = MCP `list_devices` + `ws_url` | agent 用自己的 VNC 客户端，不学新 API | 中 |
| D9 | ~~设备密码存 hub，设备 token 8 字符~~ **3.0 已作废** | 浏览器免密、可吊销；受 VNC auth 限制 | 中 |
| D10 | 单二进制、YAML/JSONL、React 前端、无 DB | 运行时仍是一个二进制；前端只在构建时要 Node | 中 |
| D11 | Docker 首选 host network | 只为日志里的真实客户端 IP；端口只有一个，bridge 不再缺功能 | 低 |
| D12 | v2 从 main 切普通分支，删 v1 目录 | 保留历史；不做 orphan | 低 |
| D13 | macOS + Android 先打通 | 覆盖 Apple RFB 变体和最难移动端 | 低 |
| D14 | v0.1 不做缩略图/录像/OCR | 这些会把 hub 变成 RFB 客户端 | 中 |
| D15 | macOS 用账户认证（Apple DH type 30） | legacy VNC 密码没有用户身份，会落到 loginwindow，画面冻结且无法输入 | 中 |

标了「3.0 已作废」的条目只在历史语境里成立：3.0 之后每台设备都是 `/ws/vnc/<id>`，hub 只监听
一个 HTTP 端口，管理员 token 是唯一密钥。

### D1 — 自托管，不做公网转发/中继

- **决定**：hub 由用户自己托管，部署在用户能控制的网络里；产品不做 relay、NAT 穿透、公网转发、内置 VPN。设备可以是任意 hub 能到达的 VNC 端点，地址由用户添加。
- **为什么**：公网转发意味着把用户的屏幕流量引到我们的基础设施上：带宽、滥用、DDoS、法律、运维、身份、计费都不是一个小 hub 能承担的；多绕一层中继在延迟和隐私上也更差。让 hub 直达设备、由用户自己的 VPN/防火墙/隧道提供边界，性能更好、攻击面更小。
- **放弃的方案**：公网 relay、反向隧道、内置 WireGuard、端口映射助手。
- **改它时必须同时判断**：认证强度（VNC 的 8 字符密码不够）、TLS/证书、滥用/限流、身份/多租户、审计/合规、DDoS、补丁/升级、运维责任、计费。不是加一个 `--public` 开关就行。
- **可逆性**：极高成本；一旦做公网转发，产品性质就变了。建议永远把网络路径留给用户。

### D2 — 设备协议只用 VNC/RFB

- **决定**：设备端只用各平台现成 VNC server；不写任何原生客户端。
- **为什么**：标准 server 现成；省掉签名、公证、打包、权限引导、升级；agent 生态有大量 VNC 客户端/库。
- **放弃的方案**：保留 v1 的 Android/macOS/Windows/Linux agent；RDP；SSH-only。
- **改它时必须同时判断**：是否要语义 UI 树（VNC 没有）、Android 零点击、各平台 server 的坑、客户端分发/更新/安全。加一个自研客户端，就是重新背上 v1 的包袱。
- **可逆性**：极高；若必须做，作为独立可选组件，不进核心。

### D3 — Hub 只做聚合网关，不做 RFB 客户端

- **决定**：hub 不实现 framebuffer 解码/输入合成；只做目录、认证、握手、转发。
- **为什么**：解码 Tight/ZRLE/JPEG、像素格式、键盘布局、坐标缩放是重复造轮子；代码 5k+；agent 自带客户端时根本不需要。
- **放弃的方案**：完整 RFB 客户端 + MCP `screenshot/click`；Guacamole 式服务端重编码。
- **改它时必须同时判断**：agent 是否有 VNC 客户端；是否需要缩略图/录像/OCR；CPU/带宽；库的维护状态（`go-vnc` / `libvncclient`）。
- **可逆性**：高；可以加独立的 CLI/适配器，不动核心。

### D4 — 两个前门：WSS 给浏览器，TCP 给 agent

> **已作废（3.0）**：agent 也走 WSS —— 同一个 `/ws/vnc/<id>`。只有 VNC 客户端不支持 WebSocket
> 时，才在 agent 本机用 `websocat` 桥一个本地 TCP 端口。

- **决定**：浏览器走 HTTPS/WSS；agent 走 RFB/TCP。
- **为什么**：浏览器必须 WebSocket；原生 VNC 客户端必须 TCP；把 RFB 包成 HTTPS 只有 noVNC 这类客户端支持，不是通用做法。
- **放弃的方案**：只做 WSS（agent 只能走 noVNC/CLI）；只做 TCP（没有网页）；HTTP API 截图（重）。
- **改它时必须同时判断**：TLS、cookie/CSRF、CORS、端口/防火墙、agent 客户端能力。
- **可逆性**：中；加/减一个前门不影响设备协议。

### D5 — 浏览器默认，裸 VNC 按设备 opt-in

> **已作废（3.0）**：没有裸 VNC 入口了。所有设备（浏览器和 agent）都走 hub 的 `/ws/vnc/<id>`，
> 端口只开一个，token 只有一个。

- **决定**：默认只开 443（浏览器）；裸 VNC 端口按设备显式开启。
- **为什么**：多数人只需要网页；避免默认铺 100 个端口；减少暴露面；Docker bridge 也更容易。
- **放弃的方案**：每台设备默认分配端口；完全不分配（agent 只能走 CLI/适配器）。
- **改它时必须同时判断**：agent 是否要直连；端口段/Docker 网络；防火墙；UI 是否展示端口。
- **可逆性**：低；配置开关。

### D6 — 裸 VNC 一设备一端口

> **已作废（3.0）**：不需要裸 VNC 入口。设备按 URL path 区分（`/ws/vnc/<id>`），hub 只有一个
> 监听端口；agent 用支持 WebSocket 的 VNC 客户端，或者在本机用 `websocat` 桥一个本地 TCP 端口。

- **决定**：raw VNC 每台设备一个 hub 端口（5901–5999）。
- **为什么**：RFB 没有 `Host`/SNI，所有 server 开场白相同，无法在握手前路由；端口是最透明的区分方式。
- **放弃的方案**：VNC repeater（客户端要支持）、WSS path（不是裸 VNC）、自定义前缀（不是裸 VNC）、一设备一 IP（macvlan，复杂）。
- **改它时必须同时判断**：端口上限、Docker 发布范围、防火墙规则、mDNS、UI 隐藏细节、客户端连接串。
- **可逆性**：中；抽象成 `Endpoint`，换分配策略不影响 UI/认证。

### D7 — L7 握手网关为核心，L4 direct 为模式

- **决定**：核心是 L7 握手网关（浏览器侧 `None`、agent 侧 VNC token、上游真实密码）；每设备可切 `mode: direct` 退回 L4。
- **为什么**：浏览器免密、密码不下发、token 可吊销、连接级审计；只碰握手，握完就是盲管道，仍然是薄层。
- **放弃的方案**：纯 L4（简单但密码散落）、浏览器 URL 带密码（泄漏）、完整 RFB 客户端（重）。
- **改它时必须同时判断**：RFB 3.3/3.7/3.8 差异、`SecurityResult`、VeNCrypt/Apple DH、token 强度/限流、失败模式、测试覆盖。
- **可逆性**：中；每设备 `mode` 可切 L4；全局开关可回退。

### D8 — AI 接口 = MCP `list_devices` + 裸 VNC

> **部分作废（3.0）**：`list_devices` 还在，但返回的是 `ws_url`（`.../ws/vnc/<id>`），不再是
> `vnc_host/port/token`。agent 用 WebSocket VNC 客户端连它，或者本机 `websocat` 桥一下。

- **决定**：MCP 只暴露 `list_devices`，返回 `vnc_host/port/token`；不做 `screenshot/click`；CLI 后置为可选。
- **为什么**：裸 VNC 是 agent 的通用接口；不做帧协议/状态机；agent 用现成 VNC 客户端。
- **放弃的方案**：完整 MCP 动作工具、`frame_id`、控制权租约；只做 CLI；完全不做 MCP。
- **改它时必须同时判断**：agent harness 是否有 VNC 客户端/shell；是否只有 MCP tool-call；token 是否进模型上下文；是否需要统一审计/限速。
- **可逆性**：中；加可选适配器，不动核心。

### D9 — 设备密码存 hub，设备 token 8 字符

> **部分作废（3.0）**：per-device token 已移除，管理员 token 是唯一密钥。hub 仍在 `gateway`
> 模式存设备密码（配置文件 0600），这部分没变。

- **决定**：hub 存设备密码（v0.1 配置文件 0600，后续钥匙串）；每设备 8 字符 token；token 可轮换。
- **为什么**：浏览器免密；密码只存一处；token 吊销即可踢人；VNC auth 只支持 8 字节，token 也只能 8 字符。
- **放弃的方案**：不存密码（每次输入）；浏览器 localStorage；OAuth/短期证书。
- **改它时必须同时判断**：静态加密、钥匙串/Secret Service、备份、进程权限、token 爆破/限流、轮换/吊销、审计。
- **可逆性**：中；存储后端可换，协议不变。

### D10 — 单二进制、YAML/JSONL、React 前端、无 DB

- **决定**：单 Go 二进制；YAML 配置；JSONL 审计；前端是 `frontend/` 下的 Vite + React + React Router + Tailwind + shadcn/ui，构建产物 `internal/web/dist` 通过 `go:embed` 嵌进二进制；无 DB。
- **为什么**：部署仍然简单（运行时不需要 Node/npm/CDN），但 UI 用标准前端栈维护，不再手写 DOM/HTML；组件、路由、表单和无障碍都走 React 生态。Node 只在 `npm run build` / Docker 构建阶段出现。
- **放弃的方案**：继续手写静态 HTML/CSS/JS（难维护、状态同步靠 DOM）；服务端模板；SQLite、Redis；运行时从 CDN 加载前端。
- **改它时必须同时判断**：构建链（Node 版本、lockfile、CI/Docker 两阶段）、`internal/web/dist` 的嵌入/占位、Go embed 的 SPA 路由、前端依赖的许可证/供应链、设备数量/并发、多用户/权限。
- **可逆性**：中；前端是独立目录，后端 API 和 WebSocket 不变；可以换框架而不动 Go。

### D11 — Docker 首选 host network

> **已作废（3.0）**：hub 只有一个 HTTP 端口，bridge 和 host 不再有端口段上的差别，选择标准只剩
> 「是否要保留真实客户端 IP」。bridge 的代价只是日志里看到 NAT 地址，功能不缺。

- **决定**：Docker 首选 `--network host`；bridge + 端口段作为受限方案；macOS 首选原生二进制。
- **为什么**：host 保留 mDNS/源 IP/动态端口；bridge 端口段固定、mDNS 不通、源 IP 被 NAT；Docker Desktop host networking 有限。
- **放弃的方案**：只用 bridge、macvlan、K8s。
- **改它时必须同时判断**：NAS 权限、端口冲突、mDNS、源 IP、升级/重启、端口段上限。
- **可逆性**：低；只影响打包。

### D12 — v2 从 main 切普通分支，删 v1 目录

- **决定**：tag `v1-final`；从 main 切普通 `v2` 分支；删除 v1 目录；不做 orphan；不建新仓库。
- **为什么**：保留历史/issue/文档引用；删目录已得到干净树；orphan 增加操作复杂度，收益小；v1 仍在 main/tag。
- **放弃的方案**：orphan、新仓库、`legacy/` 目录。
- **改它时必须同时判断**：合并策略、CI、release tag、submodule、文档链接。
- **可逆性**：低；随时可 orphan。

### D13 — macOS + Android 先打通

- **决定**：先打通 macOS + Android；Linux/Windows 后续。
- **为什么**：macOS 是 hub 宿主且自带 server；Android 是最难/最有差异的移动端；覆盖两个极端。
- **放弃的方案**：Linux 优先、Windows 优先。
- **改它时必须同时判断**：Apple RFB 变体、Android MediaProjection 授权、Wayland VNC 后端、打包/文档。
- **可逆性**：低；只影响配方和测试顺序。

### D14 — v0.1 不做缩略图/录像/OCR

- **决定**：v0.1 不做缩略图、录像、OCR。
- **为什么**：这些都需要 hub 解码画面，破坏 D3 的薄层；先做目录/查看/控制。
- **放弃的方案**：缩略图网格、录制、OCR `read_text`。
- **改它时必须同时判断**：是否接受引入 RFB 客户端/CPU/存储；是否作为可选模块；是否影响端口/认证。
- **可逆性**：中；独立模块。

### D15 — macOS 用账户认证（Apple DH type 30）

- **决定**：macOS 设备默认用 **macOS account** 登录：hub 实现 Apple DH（security type 30），用账户短名 + 账户密码认证，再按普通 RFB 3.8 转发。legacy VNC 密码（type 2）仍保留，作为兼容路径。
- **为什么**：type 2 只有一个机器级密码，没有用户身份。当前 macOS 会把它接到 `loginwindow` 会话，表现为「能看到登录画面、时钟停在登录那一刻、键鼠没反应」，但 ping 和 5900 都正常。这不是显示器睡眠，也不是 hub 丢键。noVNC-ARD 在同一台 Mac 上正常，是因为它用 type 30/33 并以具体用户登录，然后选择 console 会话。Apple DH 虽然非标准，但 noVNC 上游 1.6.0 已实现，协议格式稳定，4096-bit DH 在 Go 里也只是一次普通模幂。
- **代价**：hub 需要存 macOS 账户密码（`config.yaml` 0600）。这比 8 字符 VNC 密码更敏感；建议专用本地账户，或只在你能控制的网络中的 hub 上使用。仍然不引入 keychain，属于 D9 的同一信任模型。
- **放弃的方案**：只支持 type 2（无法进入已登录会话）；只支持 type 33/35/36（RSA tunnel 更复杂，收益不明显）；macOS 换 TigerVNC（安装/权限/登录屏限制更多）。
- **改它时必须同时判断**：D9 的凭据模型；macOS 版本差异（type 30 的参数长度、错误码）；Remote Management 的访问列表；是否要给账户密码做静态加密/keychain；MCP 输出里的 `sign_in`/`username` 字段。
- **可逆性**：中；type 30 是独立的 `ardAuth` 路径，不影响 type 2/None；设备级 `auth: vnc` 可随时切回。
- **诊断**：设备卡片会显示 warning（MCP 输出里也有）。如果 macOS 卡片还写着 `switch Sign-in to "macOS account"`，说明它仍在用 legacy VNC 密码。

### 决策之间的耦合

- **改 D1（做公网转发）**：D7/D9 的认证强度不够，要重新设计身份/TLS/滥用防护；D11 部署也要变。这是换产品，不是加功能。
- **改 D2（加自研客户端）**：D3/D9/D14 都可能变；等于回到 v1 的复杂度。
- **改 D3（hub 做 RFB 客户端）**：D6 的端口问题可能消失（单 MCP 入口），D7 的 L7 可能多余，D9 的 token 逻辑要重写。
- **改 D5（默认裸 VNC）**：D6/D11 的端口和 Docker 方案变成主路径。
- **改 D8（做 MCP 动作）**：D3 变成重方案，D6 可能消失。
- **改 D9（不存密码）**：D7 的浏览器免密失效，UX 退回每次输入。
- **改 D15（回到 legacy VNC 密码）**：D9 的凭据模型变简单，但 macOS 只能到 loginwindow，浏览器/agent 都不可控；D6 的端口和 D7 的 L7 仍在。

### 稳定性分级

- **便宜**：UI、文档、Docker 打包、平台顺序、缩略图（作为可选模块）。
- **中等**：L7/L4、密码存储、端口分配、MCP `list_devices` 的字段、macOS Apple DH。
- **昂贵**：自托管 → 公网转发、VNC → 自研客户端、薄 hub → RFB 客户端。

---

## 1. 产品定义

**abacad = 自托管的 VNC 聚合站。**

- 设备端：各平台自己的 VNC server（macOS Screen Sharing、TigerVNC、x11vnc/wayvnc/krfb/gnome-remote-desktop、droidVNC-NG）。
- 人：浏览器打开 hub，看到设备列表，点一下就连上，noVNC 实时画面。
- agent：hub 给它一个**标准 VNC 端点**，它用自己熟悉的 VNC 客户端/库连上来，不需要学新 API。
- 我们：只做目录、入口、认证、转发。**不做客户端，不做画面解码，不做 agent 动作协议。**

### 1.1 不做

- 公网转发、NAT 穿透、relay、账号、多租户；
- 各平台原生客户端；
- 文件传输、shell、端口隧道（用 SSH/SCP）；
- 完整的 `screenshot`/`click` MCP 动作工具（除非以后证明 agent 真的没有 VNC 客户端）；
- 缩略图（v1 不做，因为抓缩略图需要一个最小 RFB 客户端；先做状态灯）。

---

## 2. 架构

```
        设备（各平台现成 VNC server）                      hub（一个 Go 二进制）                消费者
 ┌──────────────────────────┐                     ┌──────────────────────────────┐
 │ macOS Screen Sharing :5900│◀── RFB (TCP) ──────│ 目录 / 健康检查 / mDNS        │
 │ TigerVNC / x11vnc   :5900 │                     │ L7 VNC 网关 :5901, :5902 ...  │◀── 原生 VNC ── agent
 │ droidVNC-NG         :5900 │                     │ WSS ⇄ TCP :443/vnc/{device}   │◀── HTTPS/WSS ─ browser
 └──────────────────────────┘                     │ 静态 UI + noVNC              │
                                                   └──────────────────────────────┘
```

### 2.1 三条路径

**浏览器**

```
https://hub/vnc/mac-mini
  → 静态页 + noVNC
  → wss://hub/vnc/mac-mini/ws
  → hub 校验 cookie
  → hub 连 mac-mini:5900，用库存密码完成 RFB 握手
  → 对浏览器伪装成 None（浏览器不需要知道密码）
  → 握手后纯字节管道
```

**agent（推荐）**

```
vnc://hub:5901   （密码 = hub 发的 per-device token）
  → hub 在该端口监听
  → 校验 token（VNC auth 的 8 字符密码）
  → hub 连 mac-mini:5900，用真实密码完成握手
  → 握手后纯字节管道
```

agent 看到的是一个**完全正常的 VNC server**：它用任何 VNC 客户端/库都能连，不需要知道背后是 mac mini 还是 Android。

**直连（可选，给不信任 hub 存密码的人）**

```
vnc://mac-mini:5900   （密码 = 设备真实密码）
```

hub 只做目录，不碰流量。配置里 `mode: direct` 即可。

### 2.2 为什么 raw VNC 需要「一设备一端口」

> **已作废（3.0）**：3.0 不再提供 raw VNC 入口，所以这个路由问题不存在了 —— 每台设备是
> `/ws/vnc/<id>`，WebSocket 握手里有 URL path。下面的分析只解释当年为什么要一设备一端口。

RFB 没有 `Host` 头、没有 SNI、没有 URL 路由；所有 server 的开场白都是 `RFB 003.008\n`。所以在一个 TCP 端口上，hub 无法在握手前知道客户端想连哪台设备。可选方案：

1. **一设备一端口**（`5901`, `5902`, ...）——最简单，仍然是裸 VNC。推荐。
2. 一设备一个 IP alias / mDNS 子域名——也行，但配置更麻烦。
3. 自定义握手前缀——不再是裸 VNC，放弃。

浏览器没有这个问题：WSS 有 URL path，`/vnc/{device}` 天然路由。

### 2.3 单入口的价值：hub 就是网络里的 VNC bastion

转发真正的好处不是「省一个客户端」，而是**拓扑收敛**：

```
你网络里的浏览器 / agent ──▶ hub:443（HTTPS/WSS）
                              hub:5901..5999（VNC）
                                   │
                                   ▼
                      设备放在另一个网段，只允许 hub 的 IP 连 5900
```

- 设备不需要对每台机器开 5900；只信任 hub 一个来源。
- 想换设备、加设备：只改 hub 的目录；外面的入口不变。
- 想审计/限流/吊销：只在 hub 一个点做。
- hub 自己变成单点和高价值目标：最小权限、只监听需要的接口、token 可轮换、日志脱敏。

**范围边界：网络路径是用户的。** 用户想把 hub 暴露到公网、跨网访问，或通过 VPN、端口映射、云网关接入，那是用户自己的网络决策；我们不实现公网转发、不托管中继，也不替用户操作。产品只保证：hub 能连到用户添加的任意 VNC 端点，浏览器/agent 能连到 hub。

这不是「单入口」和「不自建公网转发」的矛盾：**单入口是拓扑收敛；网络怎么进来，是用户网络层的事，不是 hub 的功能。**

### 2.4 Docker 与端口：100 个端口行不行？

> **已作废（3.0）**：3.0 只监听一个 HTTP 端口，bridge 网络下发布 `8787:8787` 就够，没有端口段。
> host 与 bridge 的差别只剩「是否保留真实客户端 IP」。下面的端口段分析保留作历史记录。

**行。** 一个进程监听 100 个端口没有任何问题：Go 里就是 100 次 `net.Listen`，每个 listener 一个 goroutine，内存和 fd 开销都很小。端口本身不是稀缺资源。

Docker 下有三条路：

| 部署方式 | 端口怎么开 | mDNS | 动态加设备 | 说明 |
|---|---|---|---|---|
| **`--network host`（推荐）** | 容器直接绑 `5901..5999`，不用 `-p` | ✅ | ✅ | Linux / NAS 首选；源 IP 保留；动态 listen 立即可达 |
| **bridge + 端口段** | `-p 5901-5999:5901-5999` | ❌ | 只能在发布范围内 | 每个端口一个 docker-proxy；范围在容器创建时固定；源 IP 被 NAT |
| **原生二进制** | 直接绑 | ✅ | ✅ | macOS 首选；Docker Desktop 的 host networking 支持有限 |

Compose 大致长这样：

```yaml
services:
  abacad:
    image: ghcr.io/.../abacad:latest
    network_mode: host          # Linux / NAS
    volumes:
      - ./config:/config
    # 如果只能用 bridge：
    # ports:
    #   - "443:443"
    #   - "5901-5999:5901-5999"
```

几个实际约束：

- **端口段是容器启动时固定的。** bridge 模式下发布 `5901-5999`，之后动态加到 6000 就连不进来；host 模式没有这个问题。
- **bridge 模式 mDNS 不通**：多播不跨 Docker bridge，设备发现会退化；用 host 网络或手动添加。
- **不是每台设备都需要一个裸 VNC 端口。** 浏览器只走 `443` + WSS path；裸 VNC 端口是给 agent 的，按设备 opt-in。默认可以只开 443，用户明确要 agent 直连时再分配端口。
- **「监听一个 IP 段」**：进程只能绑一个具体 IP（或 `0.0.0.0`），不能绑一个 CIDR。IP 用 macvlan 给容器一个独立 IP；设备之间还是靠端口区分。
- 99 个端口（5901–5999）对家庭/实验室足够；不够就扩到 6000–6099，host 模式下不用重建容器。

---

## 3. 三种厚度，选中间那个

| | A. 纯目录 + 直连 | **B. L7 聚合网关（推荐）** | C. 完整 RFB 客户端 + MCP 动作 |
|---|---|---|---|
| 设备暴露 | 每台 5900 对其他机器开 | 只对 hub 开 | 只对 hub 开 |
| 浏览器 | 每台单独 noVNC，或没有 | 统一站点，点开即看 | 统一站点 |
| agent | 直连设备，自己拿密码 | 连 hub，用 hub token | 调 MCP `screenshot/click` |
| 密码 | 散在客户端 | 集中在 hub，可吊销 | 集中在 hub |
| 代码量 | ~500 行 | **~1.5–2.5k 行** | ~5k 行 + RFB 客户端维护 |
| 风险 | 设备全暴露、密码散落 | 握手实现要小心 | 重复造 VNC 客户端，协议坑全吃 |

**推荐 B。** A 太散，C 太重；B 是「聚合站」这个词真正对应的形态：一个入口、一套认证、一个页面，但底层还是标准 VNC。

---

## 4. AI 接口：裸 VNC + 一个目录工具

### 4.1 主接口就是 VNC 本身

MCP 只暴露一个很小的工具：

```text
list_devices() -> [
  { id, name, platform, online, width, height,
    vnc_host, vnc_port, token, mode }
]
```

agent 拿到 `vnc_host:vnc_port` 和 `token`，用它的 VNC 客户端连。它不需要学 `observe`、`frame_id`、`act` 这些新规则——VNC 客户端本来就会截屏、点击、输入。

如果 agent 的 harness 没有 VNC 客户端，再加一个**可选的 CLI**（不是核心）：

```bash
abacad shot  mac-mini -o /tmp/frame.png
abacad click mac-mini 640 400
abacad type  mac-mini "hello"
abacad key   mac-mini ctrl+c
abacad watch mac-mini --out frames/
```

CLI 内部用一个 VNC 客户端库（`go-vnc` 或 cgo `libvncclient`），连的还是同一个 `hub:5901`。这样：

- 有 VNC 客户端的 agent：直接用，零学习。
- 只有 shell 的 agent：用 CLI，仍然是 VNC 语义。
- 只有 MCP 工具、没有 shell 的 agent：才需要可选的 `screenshot/click` MCP 适配器——这是最后手段，不是核心。

### 4.2 token 还是真实密码？

- **token 模式（推荐）**：hub 给每台设备发一个 8 字符 token，agent 用 token 连 hub；hub 用真实密码连设备。真实密码永远不出 hub；吊销 token 立即生效。
- **直连模式**：agent 拿真实密码直连设备，hub 不碰流量。适合「hub 不想存密码」的人。
- VNC auth 只支持 8 字节密码，token 也只能 8 字符；这是协议限制，不是设计选择。所以 token 要随机、可轮换，并在 hub 侧做连接限流/失败锁定。

### 4.3 为什么不做 frame_id / 动作契约

上一版设计了一个很完整的 `observe → frame_id → act → 新 frame` 契约。它解决的是「agent 通过我们的 API 操作」时的安全问题。但如果 agent 直接说 VNC：

- VNC 客户端自己就在做「收一帧 → 点一下 → 再收一帧」；
- 坐标、缩放、键盘布局是客户端的事，不是 hub 的事；
- hub 没有画面，就没有「基于旧帧乱点」的问题需要它来防；
- 真正的并发问题（两个 agent 抢鼠标）用**连接级互斥/租约**就够了，不需要帧协议。

这就是「薄」的核心：**把语义留给 VNC 客户端，把路由和认证留给 hub。**

### 4.4 Agent 在线上到底走什么协议？

**RFB over TCP。** 默认 5900；经过 hub 就是 `hub:5901`、`hub:5902`。一次典型的连接：

```
agent 的 VNC 客户端                              hub / 设备
  TCP connect hub:5901          ───────────────▶
                                ◀───────────────  RFB 003.008\n
  RFB 003.008\n                 ───────────────▶
                                ◀───────────────  安全类型 [2]（VNC auth）
  select 2                      ───────────────▶
                                ◀───────────────  16 字节 challenge
  DES(token 或密码)              ───────────────▶
                                ◀───────────────  SecurityResult 0
  ClientInit(shared)            ───────────────▶
                                ◀───────────────  ServerInit(w,h,pixel format,name)
  SetEncodings / UpdateRequest  ───────────────▶
                                ◀───────────────  FramebufferUpdate
  PointerEvent / KeyEvent       ───────────────▶
```

之后就是持续的 `FramebufferUpdateRequest` / `FramebufferUpdate` / 输入事件。**这套东西不是 agent 自己实现的**，是它调用的 VNC 客户端库实现的。

按 agent 的 harness 能力分三种情况：

| agent 有什么 | agent → hub | hub → 设备 | 说明 |
|---|---|---|---|
| VNC 客户端/库 | **RFB/TCP** | RFB/TCP | 最标准；hub 可以 L4 透明转发，或 L7 换 token |
| 能跑 shell | CLI（内部是 VNC 库） | RFB/TCP | CLI 仍是 RFB 语义；适合 Codex/Claude Code 这类 |
| 只有 MCP 工具调用 | MCP（JSON-RPC） | RFB/TCP | 这时 hub 必须内置 RFB 客户端，也就是重方案 C |

几个容易混的点：

- **MCP 不是屏幕协议。** 它是工具调用协议；如果 agent 只有 MCP，hub 就得替它说 RFB（解码画面、合成输入）。
- **noVNC 的 WebSocket 不是 VNC 协议。** 那是浏览器为了穿过 HTTP 世界加的运输层；原生 VNC 客户端走 TCP。
- **L4 和 L7 的线上协议都是 RFB。** 区别只在认证在哪终止：L4 是设备直接认证 agent；L7 是 hub 认证 agent（token），再用真实密码认证设备。
- **agent 说「调用 VNC」时，准确说是「调用一个 VNC 客户端」**；线上才是 RFB。

---

## 5. 认证与暴露面

### 5.1 hub 侧

- 首次启动生成 32 字节 admin token，打印一次；配置文件 0600。
- 浏览器：`?token=` 换 HttpOnly + SameSite=Strict cookie。
- MCP：`Authorization: Bearer <admin token>`；`list_devices` 返回的 per-device token 只能用于 VNC 端口，不能调管理 API。
- 默认监听 `0.0.0.0:8787`；建议用 `--listen <IP>` 限定到一张网卡。产品不做公网转发或中继；hub 放在哪里、怎么被访问，是用户网络层的事。
- 不做来源 IP 白名单：用户可能从 VPN / Tailscale 进来（`100.64/10` 等不是 RFC1918），那属于用户的网络层。hub 的边界是**监听地址 + token**。

### 5.2 设备侧

- 设备 5900 的防火墙只允许 hub IP：
  - Linux `ufw allow from <hub-ip> to any port 5900 proto tcp`
  - Windows 入站规则限定 hub IP
  - macOS 应用防火墙 / pf
- 支持 VeNCrypt 的设备优先 TLS；macOS Screen Sharing 不支持，只能 VNC 密码 + 受控网络兜底。
- 公网转发不在产品范围；这是用户自己的网络决策。我们只在文档里说明「设备 5900 只对 hub 开」。

### 5.3 HTTPS 的位置

- 浏览器这段必须 HTTPS/WSS：保护 cookie、剪贴板、画面。
- hub → 设备这段默认明文 RFB；如果设备支持 VeNCrypt，网关可以上游用 TLS，浏览器侧仍然是 None。v0.1 可以先只支持 VNC auth，VeNCrypt 放 v0.2。
- agent → hub 这段默认明文 TCP；放在可信网络里可接受；要加密就用 WireGuard/Tailscale 包一层，或者以后做 VeNCrypt 下游。

---

## 6. 用户流程

> **已作废（3.0）**：流程里「分配一个 hub 端口 + 一个 per-device token」「复制 `vnc://hub:5901`」
> 已经不成立。现在是：添加设备只存 `host:port` 和密码，hub 返回 `ws_url`
> （`.../ws/vnc/<id>`），浏览器和 agent 都用它 + 管理员 token。

### 加一台设备

1. 在设备上开原生 VNC server，设一个密码（macOS 系统设置、TigerVNC service、droidVNC-NG Start）。
2. hub 页面「添加设备」：名字、`host:port`、密码。不浏览 mDNS，没有待认领列表。
3. hub 检查：TCP 连上、读完 RFB banner 判断在线；添加/编辑时再检查 security types，给出认证警告。
4. 保存；分配一个 hub 端口（`5901`）和一个 token。

### 人看

1. 打开 `https://hub.local`。
2. 设备卡片显示在线/离线、分辨率、平台。
3. 点「查看」→ noVNC 全屏；可看可控制；不需要输密码。
4. 点「复制 VNC 地址」→ `vnc://hub.local:5901`，给 agent 或自己的 VNC 客户端。

### agent 用

1. agent 调 MCP `list_devices`（或读 hub 的 `/api/devices`）。
2. 拿到 `vnc_host`、`vnc_port`、`token`。
3. 用任意 VNC 客户端连接；之后就是标准的 RFB 交互。
4. 如果 agent 没有 VNC 客户端，需要一个可选适配器（v0.2 再评估，不进核心）。

### 状态与发现

- 不主动探测；只在真实会话认证后标记在线（后续决定，见 `health` 移除）。mDNS 也完全移除。
- 不浏览 `_rfb._tcp`；只广播 hub 自己的 `_abacad._tcp`。
- hub 自己广播 `_abacad._tcp`，方便 agent/人发现。

---

## 7. 代码结构

```text
cmd/abacad/
  main.go              # serve | add | list | doctor | mcp | token
internal/
  config/              # YAML + env
  inventory/           # 设备 CRUD、端口分配、健康检查
  gateway/             # L7 RFB 握手 + 双向管道（核心，~500 行）
  wsproxy/             # noVNC WSS ⇄ TCP
  web/                 # 静态 UI + noVNC（go:embed）
  mcp/                 # 只有 list_devices
  keys/                # 钥匙串（可选）
  audit/               # 连接级 JSONL（可选）
web/
  index.html
  app.js
  style.css
  novnc/
docs/
  device-setup/*.md
  security.md
  ai.md
```

依赖：`coder/websocket`、`yaml.v3`、官方 MCP SDK（只为 `list_devices`）、noVNC 资源。核心**不需要 RFB 客户端库**。

规模估计：

| 部分 | 行数 |
|---|---|
| config + inventory + 端口分配 | ~400 |
| L7 握手网关 | ~500 |
| WSS 代理 | ~150 |
| Web UI + 静态资源 | ~400 |
| mDNS + 健康检查 | ~200 |
| MCP `list_devices` | ~150 |
| **核心合计** | **~1.8k** |
| 可选 CLI（VNC 客户端） | ~800–1200 |

---

## 8. 路线图

### Phase 0 — 一天验证

- [ ] 写 100 行 L4 转发：`hub:5901` → `mac:5900`，用 TigerVNC/RealVNC 连上。
- [ ] noVNC + 最小 WSS ⇄ TCP 代理，浏览器能看。
- [ ] 读设备 banner 做在线检测。
- [ ] 确认 macOS Screen Sharing、droidVNC-NG 都能被标准 VNC 客户端连。
- [ ] 记录：首帧时间、1080p 帧大小、点击延迟。

**Go/No-Go**：标准 VNC 客户端 + noVNC 都能工作，就没有协议风险；剩下的是工程。

### Phase 1 — 聚合站 MVP（3–5 天）

- [ ] 配置、设备 CRUD、端口分配、token。
- [ ] 静态 UI：设备卡片、添加、在线状态、内嵌 noVNC。
- [ ] WSS 代理 + cookie 认证。
- [ ] L7 握手网关：浏览器侧 None、agent 侧 VNC token、上游真实密码。
- [x] mDNS 完全移除：不广播、不发现、无待认领列表。
- [x] 设备卡片的 warning（替代 CLI doctor）。
- [ ] launchd / systemd 示例。

**验收**：WAN 拔掉，浏览器点开两台设备都能看；agent 用 RealVNC/TigerVNC 连 `hub:5901` 能操作；设备 5900 除 hub 外不可见。

### Phase 2 — AI 入口与打磨（3–5 天）

- [ ] MCP `list_devices` + `/api/devices`。
- [ ] 可选：`abacad shot/click/type/key/watch` CLI。
- [ ] token 轮换（UI）、连接审计；失败锁定已有基础版。
- [ ] 设备 setup 文档（macOS / Linux X11 / Wayland / Windows / Android）。
- [x] Docker：GHCR 多架构镜像（amd64/arm64）、`compose.yaml`（`--network host`）与 `compose.bridge.yaml`、环境变量配置、`/healthz` healthcheck。
- [ ] Homebrew tap。
- [ ] 浏览器 view-only、复制 VNC 地址、二维码（可选）。

**验收**：agent 从 `list_devices` 到用 VNC 客户端完成一个 5 步任务，全程不需要人教它新 API。

### Phase 3 — 只有真需要才做

- 缩略图（需要最小 RFB 客户端）；
- 多 agent 租约/排队；
- VeNCrypt 上游 TLS；
- 可选的 `screenshot/click` MCP 适配器（给没有 VNC 客户端的 agent）；
- 录像/回放。

---

## 9. 风险

| 风险 | 影响 | 对策 |
|---|---|---|
| VNC auth 只有 8 字符 DES | 中高 | 网络隔离 + 防火墙 + token 轮换 + 失败锁定；支持 VeNCrypt 的设备优先 TLS |
| L7 握手实现有坑（3.3/3.7/3.8、SecurityResult、VeNCrypt） | 中 | 只支持 3.8 + VNC auth/None；先跑通 TigerVNC/macOS/Android；VeNCrypt 后置 |
| macOS Screen Sharing 是 Apple 变体 | 中 | 已实现 type 30 账户认证；不行再评估 type 33 或 TigerVNC |
| 一设备一端口，端口多 | 低 | 端口池 5901–5999；UI 隐藏细节；文档写清楚 |
| agent 没有 VNC 客户端 | 中 | 可选 CLI；再不行才做 MCP `screenshot/click` 适配器 |
| Android 10+ 每次开机授权 | 中 | 文档给 `adb appops`；或保留极小自研 App（唯一例外） |
| GNOME Wayland VNC 后端默认关闭 | 低 | 文档给 `grdctl vnc enable`；备选 KDE krfb/X11 |
| 两个 agent 抢鼠标 | 中 | 连接级互斥/租约；后连接的提示「设备忙」 |
| hub 成为单点/高价值目标 | 中 | 只监听需要的接口、token、最小权限、审计；不存不必要的数据 |
| 用户以为「HTTPS 就安全了」 | 中 | 文档写清楚：HTTPS 只保护浏览器段，设备段仍是 RFB |

---

## 10. 需要拍板

1. **L7 网关还是纯 L4？**
   建议 L7。多约 500 行，换来统一入口、密码不下发、token 可吊销。纯 L4 更薄，但密码散落、设备全暴露。

2. **AI 用 raw VNC + `list_devices`，还是也做 CLI？**
   建议先 raw VNC + `list_devices`；CLI 作为 Phase 2 的可选件。不要一上来做 MCP 动作工具。

3. **hub 存不存设备密码？**
   建议存（钥匙串），否则浏览器每次要输密码，聚合站的便利性就没了。

4. **第一台打通的设备？**
   建议 macOS + Android：一个验证 Apple 的 RFB 变体，一个验证最难的移动端。

5. **仓库怎么切？**
   建议 `git tag v1-final` + `git checkout --orphan v2`，同仓库孤立分支。

---

## 附录：为什么不做上一版的重方案

上一版计划是「hub 作为完整 RFB 客户端 + MCP `observe/click/type` + frame_id + 控制权租约」。它能给 agent 最好的 ergonomics，但：

- 需要解码 Tight/ZRLE/JPEG、处理像素格式、键盘布局、坐标缩放；
- 需要维护一个 VNC 客户端库（纯 Go 的 `go-vnc` 年久失修，cgo 的 libvncclient 增加构建复杂度）；
- 和「设备端用现成 VNC 客户端」的初衷矛盾——等于把客户端搬进 hub；
- 代码量 5k+，而薄版只要 1.8k。

**结论：先把薄版做出来。** 如果真实使用中发现 agent 确实没有 VNC 客户端，再加一个独立的、可选的 `abacad` CLI 或 MCP 适配器；不要把它写进核心。

---

*薄版的核心不是「功能少」，而是「把语义留给标准客户端，把路由和认证留给我们」。*

---

## 14. 实现状态（v0.1，2026-11-21）

### 已完成

- L7 RFB 握手网关：浏览器侧 `None`、agent 侧 VNC token、上游真实密码注入。
- 每设备裸 VNC 监听（`gateway` / `l4`）；`direct` 仅目录。
- noVNC 1.6.0 已 vendored 并嵌入二进制。
- 只广播 `_abacad._tcp`；不浏览 `_rfb._tcp`。
- MCP `list_devices`（官方 Go SDK，`POST /mcp` Streamable HTTP）。
- 一个 `serve` 命令（也是默认命令）；设备管理全部在 Web UI。
- 单二进制、YAML 配置 0600；前端 Vite + React + Tailwind + shadcn，构建产物嵌入二进制（运行时无 Node）。
- Dockerfile（推荐 host network）、Makefile、CI。
- 单元测试：RFB 握手（VNC/None、错误 token）、WebSocket 浏览器路径、config、inventory。
- 对真实 x11vnc 的端到端验证：握手、token、ServerInit、Raw framebuffer。

### 尚未完成（有意或待办）

- **TLS 自签/本地 CA**：`serve` 已支持 `--tls-cert` / `--tls-key`，但没有自动生成自签证书或本地 CA 的工具；用户需自备证书或前置反向代理。
- **OS 钥匙串**：设备密码目前在 `config.yaml`（0600），D9 的 keychain 迁移未做。
- **view-only / 输入过滤**：浏览器目前可控制；没有服务端只读模式。
- **缩略图 / 录像 / OCR**：按 D14 不做；它们需要 RFB 客户端。
- **多 agent 租约**：目前没有连接级互斥；两个 agent 可以同时连。
- **MCP Streamable HTTP**：已改为 `/mcp`（会话 + JSON、Bearer admin token）；stdio 命令已移除。
- **UI 编辑**：已接入 `PATCH /api/devices/{id}`；添加、编辑、删除、打开都在网页里；没有 probe/Check。
- **token 轮换**：还没有 UI/命令；admin token 与设备 token 目前都需要改配置后重启。
- **真实设备矩阵**：x11vnc 已端到端验证；一台真实 macOS Screen Sharing 已验证 banner、security types（`RFB 003.889`，`[30 33 36 2 35]`）和 type 30 DH 参数（generator 5，4096-bit prime）。macOS 账户认证的完整控制待用真实账户做一次端到端确认；droidVNC-NG、Windows TigerVNC 待实测。
- **mDNS 已完全移除**：设备一律手动添加；Docker bridge/host 都不影响这一点。

### 与原计划的偏差

- D12：实际用普通 `v2` 分支（从 main 切出）并删除 v1 目录，没有用 orphan；历史保留，树是干净的。
- D7：`direct` 是 catalog-only；盲转发是 `l4`，两者分开。
- D9：v0.1 先用配置文件存密码，钥匙串后置；这是有意的临时简化。

### 3.0 的偏差（2026-11-26）

- **单端口取代端口段。** hub 只监听 `ABACAD_LISTEN` 一个 HTTP 端口；所有设备（浏览器和
  agent）都走 `/ws/vnc/<id>`，管理员 token 是唯一密钥。D5、D6、D11 和 §2.2、§2.4 因此作废。
- **per-device token 移除。** 旧的设备 token 字段在升级时被丢弃，不再接受。
- `l4` 和 `direct` 保留：前者仍是盲转发（客户端回答设备自己的密码），后者仍是只做目录。
