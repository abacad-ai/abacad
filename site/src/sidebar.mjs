// Sidebar labels are written in English and translated through Starlight's
// `translations` map. Slugs are relative to the docs collection root; Starlight
// prefixes the active locale, so the same config drives `/docs/...` and
// `/zh/docs/...`.
export const sidebar = [
  {
    label: "Start here",
    translations: { "zh-CN": "开始" },
    items: [
      {
        label: "What is abacad",
        translations: { "zh-CN": "abacad 是什么" },
        slug: "docs",
      },
      {
        label: "Getting started",
        translations: { "zh-CN": "快速开始" },
        slug: "docs/getting-started",
      },
      {
        label: "Install",
        translations: { "zh-CN": "安装" },
        slug: "docs/install",
      },
    ],
  },
  {
    label: "Devices",
    translations: { "zh-CN": "设备" },
    items: [
      {
        label: "macOS",
        translations: { "zh-CN": "macOS" },
        slug: "docs/devices/macos",
      },
      {
        label: "Windows",
        translations: { "zh-CN": "Windows" },
        slug: "docs/devices/windows",
      },
      {
        label: "Linux",
        translations: { "zh-CN": "Linux" },
        slug: "docs/devices/linux",
      },
      {
        label: "Android",
        translations: { "zh-CN": "Android" },
        slug: "docs/devices/android",
      },
      {
        label: "iOS & iPadOS",
        translations: { "zh-CN": "iOS 与 iPadOS" },
        slug: "docs/devices/ios",
      },
    ],
  },
  {
    label: "For agents",
    translations: { "zh-CN": "给 agent" },
    items: [
      {
        label: "MCP",
        translations: { "zh-CN": "MCP" },
        slug: "docs/agents/mcp",
      },
      {
        label: "Agent skill",
        translations: { "zh-CN": "Agent 技能" },
        slug: "docs/agents/skill",
      },
      {
        label: "WebSocket and VNC",
        translations: { "zh-CN": "WebSocket 与 VNC" },
        slug: "docs/agents/websocket",
      },
    ],
  },
  {
    label: "Operate",
    translations: { "zh-CN": "运维" },
    items: [
      {
        label: "Configuration",
        translations: { "zh-CN": "配置" },
        slug: "docs/operations/configuration",
      },
      {
        label: "Security",
        translations: { "zh-CN": "安全" },
        slug: "docs/operations/security",
      },
      {
        label: "Troubleshooting",
        translations: { "zh-CN": "排错" },
        slug: "docs/operations/troubleshooting",
      },
    ],
  },
];
