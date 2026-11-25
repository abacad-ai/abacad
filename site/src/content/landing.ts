// Every word on the two landing pages lives here. Keeping the copy in one typed
// module makes the English and Chinese pages structurally identical: a missing
// section is a type error, not a page that quietly renders half a layout.

export const en = {
  lang: "en",
  htmlLang: "en",
  skip: "Skip to content",
  meta: {
    title: "abacad — a self-hosted VNC hub for your machines and your agents",
    description:
      "Connect the VNC servers your machines already run. Open any of them in a browser, and hand the same stream to an agent. Self-hosted, no cloud, no relay, no software on the devices.",
  },
  nav: {
    how: "How it works",
    devices: "Devices",
    agents: "For agents",
    docs: "Docs",
    github: "GitHub",
    cta: "Get started",
    switchLabel: "中文",
    switchHref: "/zh/",
    switchLang: "zh-CN",
  },
  hero: {
    eyebrow: "SELF-HOSTED VNC HUB · NO CLOUD · NO RELAY",
    title: "Your machines, one quiet place.",
    lead: "Every screen you own already speaks VNC. abacad gives them one home: a browser console for you, the same stream for an agent, one port and one token. No software on the devices. No cloud in the middle.",
    primary: "Start in Docker",
    primaryHref: "/docs/getting-started/",
    secondary: "How it works",
    secondaryHref: "#how",
    note: "One port · one admin token · no relay",
    commandLabel: "Run the hub",
    command:
      "docker run -d --name abacad --network host \\\n  -v abacad-config:/config \\\n  ghcr.io/abacad-ai/abacad:latest",
    commandNote: "Then open http://localhost:8787. The admin token is in `docker logs abacad`.",
    copy: "Copy",
    copied: "Copied",
  },
  console: {
    hub: "abacad",
    address: "192.168.1.10:8787",
    railLabel: "MACHINES",
    devices: [
      { name: "Mac mini", meta: "macOS account", state: "online" },
      { name: "Studio", meta: "x11vnc · :5900", state: "idle" },
      { name: "Pixel 9", meta: "droidVNC-NG", state: "online" },
      { name: "iPad", meta: "via iosvnc", state: "idle" },
    ],
    add: "Add device",
    screenLabel: "Mac mini — live",
    screenLines: [
      "abacad@mac-mini ~ % ./hub status",
      "hub      https://abacad.local",
      "devices  4",
      "mode     gateway",
      "agent    mcp · connected",
      "~ %",
    ],
    footLeft: "gateway mode",
    footRight: "RFB · one WebSocket",
  },
  worksWith: {
    label: "WORKS WITH THE VNC SERVER YOU ALREADY HAVE",
    items: [
      "macOS Screen Sharing",
      "TigerVNC",
      "x11vnc",
      "wayvnc",
      "droidVNC-NG",
      "iosvnc",
    ],
  },
  why: {
    eyebrow: "WHY A HUB",
    title: "One place for every screen you own.",
    lead: "Not another remote-desktop app. A small, self-hosted gateway between the VNC servers you already trust and the clients that need them.",
    cards: [
      {
        icon: "device",
        title: "Nothing to install on the device",
        body: "Use the server the OS already ships, or the standard one for the platform. abacad never asks you to put an agent on a machine.",
      },
      {
        icon: "port",
        title: "One port. One token.",
        body: "The console, the API, MCP and every device stream share a single HTTP port and one admin token. One reverse proxy covers the whole hub.",
      },
      {
        icon: "stream",
        title: "The same endpoint for a person or an agent",
        body: "A browser gets noVNC. An agent gets the same WebSocket, plain RFB. There is no second screen API to learn or to keep in sync.",
      },
    ],
  },
  how: {
    eyebrow: "HOW IT WORKS",
    title: "The hub terminates the handshake, then gets out of the way.",
    lead: "In gateway mode abacad signs in to the device for you, so neither the browser nor the agent ever sees the device password. After authentication it is a blind byte pipe: it never decodes a framebuffer, never records, never synthesizes input.",
    steps: [
      {
        label: "01",
        title: "Your VNC server",
        body: "macOS Screen Sharing, TigerVNC, x11vnc, wayvnc, droidVNC-NG, or iosvnc. Keep it on the LAN.",
      },
      {
        label: "02",
        title: "abacad hub",
        body: "One WebSocket per device at /ws/vnc/<id>. One admin token. One HTTP port.",
      },
      {
        label: "03",
        title: "Browser or agent",
        body: "noVNC in the console, or any VNC client through a websocat bridge.",
      },
    ],
    note: "No discovery. No probing. A device only goes online after a real session authenticates to it.",
    noteLink: "Why that matters",
    noteHref: "/docs/#online",
    cta: "Read the architecture",
    ctaHref: "/docs/",
  },
  platforms: {
    eyebrow: "DEVICES",
    title: "Use the server the platform already has.",
    lead: "Five setup guides, one per platform. Each one covers the server, the credentials, the firewall, and the parts that usually go wrong.",
    link: "Read the guide",
    items: [
      {
        name: "macOS",
        kicker: "Screen Sharing + macOS account",
        body: "The account path attaches to the logged-in session, not the login window.",
        href: "/docs/devices/macos/",
      },
      {
        name: "Windows",
        kicker: "TigerVNC in service mode",
        body: "Includes the login screen, and a firewall rule scoped to the hub.",
        href: "/docs/devices/windows/",
      },
      {
        name: "Linux",
        kicker: "x11vnc, TigerVNC, wayvnc",
        body: "X11 and Wayland, with a systemd user unit you can keep.",
        href: "/docs/devices/linux/",
      },
      {
        name: "Android",
        kicker: "droidVNC-NG",
        body: "The standard VNC server for Android, with the MediaProjection caveat.",
        href: "/docs/devices/android/",
      },
      {
        name: "iOS & iPadOS",
        kicker: "iosvnc bridge",
        body: "No VNC server on the device. A Mac you already pair it to speaks RFB for it.",
        href: "/docs/devices/ios/",
      },
    ],
  },
  agents: {
    eyebrow: "FOR AGENTS",
    title: "Your agent already speaks VNC.",
    lead: "abacad does not invent a new device API. It exposes one MCP tool, list_devices, and returns a WebSocket per machine carrying plain RFB. Any VNC client works; the agent does not need to be taught your hardware.",
    bullets: [
      "MCP over Streamable HTTP at /mcp",
      "One tool: list_devices",
      "Or skip MCP: /skill writes a ready-made SKILL.md",
    ],
    codeTitle: "MCP client config",
    code: `{
  "mcpServers": {
    "abacad": {
      "type": "http",
      "url": "http://192.168.1.10:8787/mcp",
      "headers": {
        "Authorization": "Bearer <admin token>"
      }
    }
  }
}`,
    cta: "Set up MCP",
    ctaHref: "/docs/agents/mcp/",
    secondary: "See the skill",
    secondaryHref: "/docs/agents/skill/",
    note: "The token opens every machine. Treat it like a password.",
  },
  human: {
    eyebrow: "A HUMAN IN THE LOOP",
    title: "When the agent gets stuck, you can step in.",
    lead: "A login screen, a one-time code, a captcha, a permission prompt: some steps need a person. Open the same device in the console to watch or take over, finish the step, and let the agent carry on. No second tool, no second screen.",
    agentLabel: "AGENT",
    agentText: "Waiting at a login screen. Needs a code.",
    humanLabel: "YOU",
    humanText: "Open the same device in abacad, type the code, close the tab.",
    result: "The agent carries on. Same machine, same screen.",
    cta: "How the console works",
    ctaHref: "/docs/getting-started/",
  },
  selfHosted: {
    eyebrow: "SELF-HOSTED, BY DESIGN",
    title: "The network is the boundary.",
    lead: "abacad is not a hosted service. You run the hub, you own the path to it, and you decide who can reach it. That is also why it is fast: there is no relay in the middle.",
    cards: [
      {
        icon: "shield",
        title: "No relay, no public forwarding",
        body: "Put the hub behind a VPN or a TLS reverse proxy. abacad never opens a connection to a device on its own.",
      },
      {
        icon: "lock",
        title: "One ingress to defend",
        body: "Restrict each device's VNC port to the hub's IP. The hub is the only machine that needs to reach 5900.",
      },
      {
        icon: "reuse",
        title: "Old hardware earns its keep",
        body: "A 2015 Mac mini, a spare Android phone, a headless Linux box: if it runs a VNC server, it is a first-class machine again.",
      },
    ],
    cta: "Read the security model",
    ctaHref: "/docs/operations/security/",
  },
  faq: {
    eyebrow: "QUESTIONS",
    title: "The short answers.",
    items: [
      {
        q: "Is abacad a cloud service?",
        a: "No. It is a single Go binary you run. There is no account, no relay, and no vendor in the path. If you expose it, you do it behind your own TLS and VPN.",
      },
      {
        q: "Do I install anything on the device?",
        a: "No. Use the VNC server the platform already has, or the standard one for that platform. The one exception is iOS and iPadOS: they have no VNC server, so a paired Mac runs the small iosvnc bridge.",
      },
      {
        q: "Can a person and an agent use the same device at once?",
        a: "Yes, but treat it as one session. A human can open the same device in the console to take over a login or a prompt. Some VNC servers allow multiple viewers, others do not; one at a time is the safe default.",
      },
      {
        q: "What happens when the hub is down?",
        a: "Your VNC servers keep running. abacad is a gateway, not a dependency inside the device. Point a normal VNC client at the machine, or bring the hub back.",
      },
      {
        q: "Does it work over the internet?",
        a: "Only if you make it reachable, and you should do that with TLS and a VPN. The hub does not do NAT traversal, port forwarding, or discovery for you.",
      },
      {
        q: "Which VNC clients work with an agent?",
        a: "Any standard RFB client. Open the device WebSocket directly if the client supports WebSockets, or bridge it to a local TCP port with websocat.",
      },
    ],
  },
  final: {
    title: "One quiet place for every screen.",
    lead: "Run the hub on the machine that is always on. Connect the first device in a minute.",
    primary: "Start with Docker",
    primaryHref: "/docs/getting-started/",
    secondary: "Read the docs",
    secondaryHref: "/docs/",
    commandLabel: "Or pull the image",
    command:
      "docker run -d --name abacad --network host \\\n  -v abacad-config:/config \\\n  ghcr.io/abacad-ai/abacad:latest",
    copy: "Copy",
    copied: "Copied",
  },
  footer: {
    tagline: "A small, self-hosted VNC hub. No cloud, no relay, no account.",
    columns: [
      {
        title: "Product",
        links: [
          { label: "How it works", href: "#how" },
          { label: "Devices", href: "#devices" },
          { label: "For agents", href: "#agents" },
        ],
      },
      {
        title: "Docs",
        links: [
          { label: "Getting started", href: "/docs/getting-started/" },
          { label: "Install", href: "/docs/install/" },
          { label: "Security", href: "/docs/operations/security/" },
        ],
      },
      {
        title: "Source",
        links: [
          { label: "GitHub", href: "https://github.com/abacad-ai/abacad" },
          { label: "Releases", href: "https://github.com/abacad-ai/abacad/releases" },
          { label: "Issues", href: "https://github.com/abacad-ai/abacad/issues" },
        ],
      },
    ],
    bottom: "© 2026 abacad",
    privacy: "A static site. No cookies, no analytics.",
  },
};

export type LandingCopy = typeof en;

export const zh: LandingCopy = {
  lang: "zh",
  htmlLang: "zh-CN",
  skip: "跳到主要内容",
  meta: {
    title: "abacad — 自托管 VNC hub，给你的机器和 agent 一个入口",
    description:
      "把机器上已有的 VNC 服务接进来：在浏览器里打开任意一台，也可以把同一个画面交给 agent。自托管，无云端，无中继，设备上不用装软件。",
  },
  nav: {
    how: "怎么工作",
    devices: "设备",
    agents: "给 agent",
    docs: "文档",
    github: "GitHub",
    cta: "开始使用",
    switchLabel: "EN",
    switchHref: "/",
    switchLang: "en",
  },
  hero: {
    eyebrow: "自托管 VNC HUB · 无云端 · 无中继",
    title: "你的机器，一处安静的地方。",
    lead: "你手上的每一块屏幕都已经会说 VNC。abacad 给它们一个统一的入口：你在浏览器里打开，agent 拿到同一个画面；一个端口，一个 token。设备上不装任何软件，中间没有云端。",
    primary: "用 Docker 启动",
    primaryHref: "/zh/docs/getting-started/",
    secondary: "看看怎么工作",
    secondaryHref: "#how",
    note: "一个端口 · 一个管理员 token · 没有中继",
    commandLabel: "启动 hub",
    command:
      "docker run -d --name abacad --network host \\\n  -v abacad-config:/config \\\n  ghcr.io/abacad-ai/abacad:latest",
    commandNote: "打开 http://localhost:8787。管理员 token 在 `docker logs abacad` 里。",
    copy: "复制",
    copied: "已复制",
  },
  console: {
    hub: "abacad",
    address: "192.168.1.10:8787",
    railLabel: "机器",
    devices: [
      { name: "Mac mini", meta: "macOS 账户", state: "online" },
      { name: "Studio", meta: "x11vnc · :5900", state: "idle" },
      { name: "Pixel 9", meta: "droidVNC-NG", state: "online" },
      { name: "iPad", meta: "经 iosvnc 桥接", state: "idle" },
    ],
    add: "添加设备",
    screenLabel: "Mac mini — 在线",
    screenLines: [
      "abacad@mac-mini ~ % ./hub status",
      "hub      https://abacad.local",
      "devices  4",
      "mode     gateway",
      "agent    mcp · connected",
      "~ %",
    ],
    footLeft: "gateway 模式",
    footRight: "RFB · 一个 WebSocket",
  },
  worksWith: {
    label: "兼容你已经在用的 VNC 服务",
    items: [
      "macOS 屏幕共享",
      "TigerVNC",
      "x11vnc",
      "wayvnc",
      "droidVNC-NG",
      "iosvnc",
    ],
  },
  why: {
    eyebrow: "为什么需要一个 hub",
    title: "所有屏幕，一个入口。",
    lead: "它不是又一个远程桌面 App，而是站在你信任的 VNC 服务和需要它的客户端之间的一个小小自托管网关。",
    cards: [
      {
        icon: "device",
        title: "设备上什么都不用装",
        body: "用系统自带的 VNC 服务，或用这个平台标准的那个。abacad 不会要求你往机器里塞一个 agent。",
      },
      {
        icon: "port",
        title: "一个端口，一个 token",
        body: "控制台、API、MCP 和所有设备的画面共用同一个 HTTP 端口和一个管理员 token。一个反向代理就能覆盖全部。",
      },
      {
        icon: "stream",
        title: "人和 agent 走同一个端点",
        body: "浏览器拿到 noVNC，agent 拿到同一个 WebSocket、同样的裸 RFB。没有第二套屏幕 API 要学，也没有两套状态要同步。",
      },
    ],
  },
  how: {
    eyebrow: "怎么工作",
    title: "hub 只负责握手，然后让路。",
    lead: "gateway 模式下，abacad 替你登录设备，浏览器和 agent 都看不到设备密码。认证完成后它就是一根盲管：不解码画面、不录屏、不合成输入。",
    steps: [
      {
        label: "01",
        title: "你的 VNC 服务",
        body: "macOS 屏幕共享、TigerVNC、x11vnc、wayvnc、droidVNC-NG，或者 iosvnc。让它留在局域网里。",
      },
      {
        label: "02",
        title: "abacad hub",
        body: "每台设备一个 WebSocket：/ws/vnc/<id>。一个管理员 token，一个 HTTP 端口。",
      },
      {
        label: "03",
        title: "浏览器或 agent",
        body: "控制台里是 noVNC；agent 用任意 VNC 客户端，或者用 websocat 桥到本地端口。",
      },
    ],
    note: "不扫描、不探测。只有真实会话认证成功，设备才会显示在线。",
    noteLink: "为什么这样设计",
    noteHref: "/zh/docs/#online",
    cta: "读读架构",
    ctaHref: "/zh/docs/",
  },
  platforms: {
    eyebrow: "设备",
    title: "用平台本来就有的 VNC 服务。",
    lead: "五篇设置指南，一个平台一篇。每篇都写清楚服务、凭据、防火墙，以及最容易卡住的地方。",
    link: "读设置指南",
    items: [
      {
        name: "macOS",
        kicker: "屏幕共享 + macOS 账户",
        body: "账户方式会附着到已登录的会话，而不是 loginwindow。",
        href: "/zh/docs/devices/macos/",
      },
      {
        name: "Windows",
        kicker: "TigerVNC 服务模式",
        body: "能覆盖登录界面，防火墙规则只放行 hub。",
        href: "/zh/docs/devices/windows/",
      },
      {
        name: "Linux",
        kicker: "x11vnc、TigerVNC、wayvnc",
        body: "X11 与 Wayland，附一个可以长期用的 systemd 用户单元。",
        href: "/zh/docs/devices/linux/",
      },
      {
        name: "Android",
        kicker: "droidVNC-NG",
        body: "Android 上标准的 VNC 服务，以及 MediaProjection 的注意事项。",
        href: "/zh/docs/devices/android/",
      },
      {
        name: "iOS 与 iPadOS",
        kicker: "iosvnc 桥接",
        body: "设备本身没有 VNC 服务；用已经配对的 Mac 替它说 RFB。",
        href: "/zh/docs/devices/ios/",
      },
    ],
  },
  agents: {
    eyebrow: "给 agent",
    title: "你的 agent 本来就会说 VNC。",
    lead: "abacad 没有发明一套新的设备 API。它只暴露一个 MCP 工具 list_devices，给每台机器一个承载裸 RFB 的 WebSocket。任何 VNC 客户端都能用，不需要教 agent 认识你的硬件。",
    bullets: [
      "MCP 走 Streamable HTTP，端点是 /mcp",
      "只有一个工具：list_devices",
      "不用 MCP 也行：/skill 直接生成一份 SKILL.md",
    ],
    codeTitle: "MCP 客户端配置",
    code: `{
  "mcpServers": {
    "abacad": {
      "type": "http",
      "url": "http://192.168.1.10:8787/mcp",
      "headers": {
        "Authorization": "Bearer <admin token>"
      }
    }
  }
}`,
    cta: "配置 MCP",
    ctaHref: "/zh/docs/agents/mcp/",
    secondary: "看看 skill",
    secondaryHref: "/zh/docs/agents/skill/",
    note: "token 能打开所有机器，当成密码保管。",
  },
  human: {
    eyebrow: "人在环路里",
    title: "agent 卡住的时候，你可以接手。",
    lead: "登录界面、一次性的验证码、captcha、权限弹窗：有些步骤必须由人来做。在控制台里打开同一台设备，看着它，或者直接接手，把这一步做完，再让 agent 继续。不需要第二个工具，也不需要第二块屏幕。",
    agentLabel: "AGENT",
    agentText: "停在登录界面，等一个验证码。",
    humanLabel: "你",
    humanText: "在 abacad 里打开同一台设备，输入验证码，关掉标签页。",
    result: "agent 继续。同一台机器，同一个画面。",
    cta: "看看控制台怎么用",
    ctaHref: "/zh/docs/getting-started/",
  },
  selfHosted: {
    eyebrow: "自托管，是设计不是妥协",
    title: "网络边界就是安全边界。",
    lead: "abacad 不是托管服务。hub 由你运行，到它的路径由你掌控，谁能访问也由你决定。这也是它快的原因：中间没有中继。",
    cards: [
      {
        icon: "shield",
        title: "没有中继，没有公网转发",
        body: "把 hub 放在 VPN 或 TLS 反向代理后面。abacad 不会主动向设备发起连接。",
      },
      {
        icon: "lock",
        title: "只有一个入口要守",
        body: "把每台设备的 VNC 端口限制为只允许 hub 的 IP。整个网络里只有 hub 需要访问 5900。",
      },
      {
        icon: "reuse",
        title: "旧设备重新上岗",
        body: "2015 年的 Mac mini、闲置的 Android 手机、没有显示器的 Linux 小主机：只要能跑 VNC 服务，它就是一台正式的机器。",
      },
    ],
    cta: "读安全模型",
    ctaHref: "/zh/docs/operations/security/",
  },
  faq: {
    eyebrow: "常见问题",
    title: "短回答。",
    items: [
      {
        q: "abacad 是云服务吗？",
        a: "不是。它是一个你自己运行的 Go 单文件。没有账号、没有中继，也没有厂商夹在中间。如果要暴露到公网，请放在你自己的 TLS 和 VPN 后面。",
      },
      {
        q: "需要在设备上装东西吗？",
        a: "不需要。用平台自带的 VNC 服务，或用该平台标准的那个。唯一的例外是 iOS 和 iPadOS：它们没有 VNC 服务，所以由一台已配对的 Mac 运行很小的 iosvnc 桥。",
      },
      {
        q: "人和 agent 能同时用一台设备吗？",
        a: "可以，但把它当成一个会话。人可以在控制台里打开同一台设备，接手登录或验证码；有些 VNC 服务允许多个查看者，有些不允。稳妥起见，一次一个。",
      },
      {
        q: "hub 挂了怎么办？",
        a: "你的 VNC 服务照常运行。abacad 只是网关，不是设备里的依赖。用普通 VNC 客户端直接连机器，或者把 hub 重新启动。",
      },
      {
        q: "能走公网吗？",
        a: "可以，但请用 TLS 和 VPN 让它可达。hub 不替你做 NAT 穿透、端口转发或设备发现。",
      },
      {
        q: "agent 能用哪些 VNC 客户端？",
        a: "任何标准 RFB 客户端。客户端支持 WebSocket 就直接打开设备的 WebSocket；不支持就用 websocat 桥到本地 TCP 端口。",
      },
    ],
  },
  final: {
    title: "所有屏幕，一处安放。",
    lead: "把 hub 跑在那台一直开着的机器上。一分钟接进第一台设备。",
    primary: "用 Docker 启动",
    primaryHref: "/zh/docs/getting-started/",
    secondary: "读文档",
    secondaryHref: "/zh/docs/",
    commandLabel: "或者直接拉镜像",
    command:
      "docker run -d --name abacad --network host \\\n  -v abacad-config:/config \\\n  ghcr.io/abacad-ai/abacad:latest",
    copy: "复制",
    copied: "已复制",
  },
  footer: {
    tagline: "一个小巧的自托管 VNC hub。无云端，无中继，无账号。",
    columns: [
      {
        title: "产品",
        links: [
          { label: "怎么工作", href: "#how" },
          { label: "设备", href: "#devices" },
          { label: "给 agent", href: "#agents" },
        ],
      },
      {
        title: "文档",
        links: [
          { label: "快速开始", href: "/zh/docs/getting-started/" },
          { label: "安装", href: "/zh/docs/install/" },
          { label: "安全", href: "/zh/docs/operations/security/" },
        ],
      },
      {
        title: "源码",
        links: [
          { label: "GitHub", href: "https://github.com/abacad-ai/abacad" },
          { label: "Releases", href: "https://github.com/abacad-ai/abacad/releases" },
          { label: "Issues", href: "https://github.com/abacad-ai/abacad/issues" },
        ],
      },
    ],
    bottom: "© 2026 abacad",
    privacy: "静态页面，不收集任何东西。",
  },
};

export const landing = { en, zh } as const;
