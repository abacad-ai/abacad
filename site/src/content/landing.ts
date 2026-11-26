// The landing page is deliberately short. The docs carry the detail; this page
// has one job: show the product in the first screen, then get out of the way.
// Both locales have the same shape, so a missing line is a type error.

export const en = {
  lang: "en",
  htmlLang: "en",
  skip: "Skip to content",
  meta: {
    title: "abacad — every screen you own, one browser tab",
    description:
      "A self-hosted VNC hub. Open your Mac, PC, Linux box or phone in the browser — or let an agent drive the same screen. No cloud, no relay, nothing installed on the device.",
  },
  nav: {
    agents: "Agents",
    docs: "Docs",
    github: "GitHub",
    cta: "Get started",
    switchLabel: "中文",
    switchHref: "/zh/",
    switchLang: "zh-CN",
  },
  hero: {
    kicker: "Self-hosted VNC hub",
    title: ["Every screen you own.", "One tab."],
    lead: "Open your Mac, PC, Linux box or phone in the browser — or let your agent drive the same screen. No cloud, no relay, nothing installed on the device.",
    primary: "Run the hub",
    primaryHref: "/docs/getting-started/",
    secondary: "Read the docs",
    secondaryHref: "/docs/",
    note: "One Docker command · one port · one token",
  },
  shelf: {
    live: "Live from one hub",
    meta: "3 machines · 23:22",
    android: {
      os: "Android",
      name: "Galaxy S10",
      src: "/screens/live-android.webp",
      width: 512,
      height: 1080,
      alt: "A Galaxy S10 home screen with Chrome, Reddit, X and YouTube, captured live over VNC.",
    },
    windows: {
      os: "Windows 11",
      name: "NUC12",
      src: "/screens/live-windows.webp",
      width: 1200,
      height: 681,
      alt: "A Windows 11 desktop with a bamboo forest wallpaper, captured live over VNC.",
    },
    debian: {
      os: "Debian",
      name: "Locked",
      src: "/screens/live-debian.webp",
      width: 1200,
      height: 681,
      alt: "A Debian lock screen showing 11:21 PM, captured live over VNC.",
    },
    caption: "Real screens, captured live from a single hub.",
    foot: "RFB · one WebSocket each",
  },
  claims: [
    {
      title: "Watch or take over.",
      body: "Any machine, any browser. Nothing to install on the device.",
    },
    {
      title: "Step in when it's stuck.",
      body: "A login, a code, a captcha — finish it on the same screen, then hand it back.",
    },
    {
      title: "Keep it yours.",
      body: "One port, one token, no relay. Your network is the boundary.",
    },
  ],
  works: {
    label: "Works with the VNC server you already run",
    items: ["macOS Screen Sharing", "TigerVNC", "x11vnc", "wayvnc", "droidVNC-NG", "iOS VNC Bridge"],
  },
  agents: {
    kicker: "For agents",
    title: "The screen your agent is missing.",
    lead: "One MCP tool, <code>list_devices</code>, returns every machine as a plain RFB WebSocket. Any VNC client can drive it — no new device API, no browser automation.",
    note: "When the agent hits a login or a captcha, open the same screen and take over.",
    codeTitle: "MCP client",
    code: `{
  "mcpServers": {
    "abacad": {
      "type": "http",
      "url": "https://hub.local/mcp",
      "headers": {
        "Authorization": "Bearer $TOKEN"
      }
    }
  }
}`,
    copy: "Copy",
    copied: "Copied",
    cta: "Set up MCP",
    ctaHref: "/docs/agents/mcp/",
  },
  final: {
    title: "One command. Then every screen.",
    lead: "Run the hub on the machine that is always on.",
    commandLabel: "Terminal",
    command:
      "docker run -d --name abacad --network host \\\n  -v abacad-config:/config \\\n  ghcr.io/abacad-ai/abacad:latest",
    copy: "Copy",
    copied: "Copied",
    docs: "Read the docs",
    docsHref: "/docs/",
  },
  footer: {
    tagline: "A self-hosted VNC hub. No cloud, no relay, no account.",
    links: [
      { label: "Docs", href: "/docs/" },
      { label: "GitHub", href: "https://github.com/abacad-ai/abacad" },
      { label: "Security", href: "/docs/operations/security/" },
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
    title: "abacad — 所有屏幕，一个标签页",
    description:
      "自托管 VNC hub。在浏览器里打开你的 Mac、PC、Linux 和手机，也可以让 agent 驱动同一个画面。没有云端，没有中继，设备上什么都不用装。",
  },
  nav: {
    agents: "给 agent",
    docs: "文档",
    github: "GitHub",
    cta: "开始使用",
    switchLabel: "EN",
    switchHref: "/",
    switchLang: "en",
  },
  hero: {
    kicker: "自托管 VNC HUB",
    title: ["所有屏幕，", "一个标签页。"],
    lead: "在浏览器里打开你的 Mac、PC、Linux 和手机，也可以让 agent 驱动同一个画面。<br>没有云端，没有中继，设备上什么都不用装。",
    primary: "启动 hub",
    primaryHref: "/zh/docs/getting-started/",
    secondary: "读文档",
    secondaryHref: "/zh/docs/",
    note: "一条 Docker 命令 · 一个端口 · 一个 token",
  },
  shelf: {
    live: "来自一个 hub 的实时画面",
    meta: "3 台机器 · 23:22",
    android: {
      os: "Android",
      name: "Galaxy S10",
      src: "/screens/live-android.webp",
      width: 512,
      height: 1080,
      alt: "通过 VNC 实时截取的 Galaxy S10 桌面，上面有 Chrome、Reddit、X 和 YouTube。",
    },
    windows: {
      os: "Windows 11",
      name: "NUC12",
      src: "/screens/live-windows.webp",
      width: 1200,
      height: 681,
      alt: "通过 VNC 实时截取的一台 Windows 11 桌面，竹子林壁纸。",
    },
    debian: {
      os: "Debian",
      name: "已锁屏",
      src: "/screens/live-debian.webp",
      width: 1200,
      height: 681,
      alt: "通过 VNC 实时截取的一台 Debian 锁屏界面，时间是 11:21 PM。",
    },
    caption: "真实屏幕，从同一个 hub 实时截取。",
    foot: "RFB · 每台一个 WebSocket",
  },
  claims: [
    {
      title: "看着，或者直接接手。",
      body: "任何机器，任何浏览器，设备上什么都不用装。",
    },
    {
      title: "卡住的时候，你来。",
      body: "登录、验证码、captcha——在同一个画面上做完，再交还给 agent。",
    },
    {
      title: "留在你自己的网络里。",
      body: "一个端口，一个 token，没有中继。网络边界就是安全边界。",
    },
  ],
  works: {
    label: "兼容你已经在用的 VNC 服务",
    items: ["macOS 屏幕共享", "TigerVNC", "x11vnc", "wayvnc", "droidVNC-NG", "iOS VNC Bridge"],
  },
  agents: {
    kicker: "给 agent",
    title: "agent 缺的那块屏幕。",
    lead: "一个 MCP 工具，<code>list_devices</code>，把每台机器变成一条裸 RFB 的 WebSocket。任何 VNC 客户端都能驱动——没有新的设备 API，也不需要浏览器自动化。",
    note: "agent 卡在登录或验证码时，你在同一个画面上接手。",
    codeTitle: "MCP 客户端",
    code: `{
  "mcpServers": {
    "abacad": {
      "type": "http",
      "url": "https://hub.local/mcp",
      "headers": {
        "Authorization": "Bearer $TOKEN"
      }
    }
  }
}`,
    copy: "复制",
    copied: "已复制",
    cta: "配置 MCP",
    ctaHref: "/zh/docs/agents/mcp/",
  },
  final: {
    title: "一条命令，接进所有屏幕。",
    lead: "把 hub 跑在那台一直开着的机器上。",
    commandLabel: "终端",
    command:
      "docker run -d --name abacad --network host \\\n  -v abacad-config:/config \\\n  ghcr.io/abacad-ai/abacad:latest",
    copy: "复制",
    copied: "已复制",
    docs: "读文档",
    docsHref: "/zh/docs/",
  },
  footer: {
    tagline: "一个自托管的 VNC hub。没有云端，没有中继，没有账号。",
    links: [
      { label: "文档", href: "/zh/docs/" },
      { label: "GitHub", href: "https://github.com/abacad-ai/abacad" },
      { label: "安全", href: "/zh/docs/operations/security/" },
    ],
    bottom: "© 2026 abacad",
    privacy: "静态页面，不收集任何东西。",
  },
};

export const landing = { en, zh } as const;
