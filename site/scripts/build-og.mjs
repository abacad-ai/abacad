// Builds the two 1200×630 social cards. Run `npm run og` after changing copy.
// The cards are committed so the site build never depends on a particular
// fontconfig or on ImageMagick being installed.
import sharp from "sharp";

const mark = `
  <g transform="translate(80 72) scale(0.62)">
    <path fill="#fd605e" d="M23 8H55A15 15 0 0 1 70 23V25H45A20 20 0 0 0 25 45V70H23A15 15 0 0 1 8 55V23A15 15 0 0 1 23 8Z"/>
    <rect x="30" y="30" width="62" height="62" rx="15" fill="#13a7f5"/>
  </g>`;

const cards = {
  "og.png": {
    eyebrow: "SELF-HOSTED VNC HUB · NO CLOUD · NO RELAY",
    lines: ["Your machines,", "one quiet place."],
    lead: ["Connect the VNC servers your machines already run.", "Open any of them in a browser — or hand the stream to an agent."],
    foot: "ONE PORT · ONE TOKEN · NO RELAY",
    family: "Helvetica Neue, Helvetica, Arial, sans-serif",
    headlineSize: 78,
    leadSize: 23,
  },
  "og-zh.png": {
    eyebrow: "自托管 VNC HUB · 无云端 · 无中继",
    lines: ["你的机器，", "一处安静的地方。"],
    lead: ["把机器上已有的 VNC 服务接进来。", "浏览器里打开，或者交给 agent。"],
    foot: "一个端口 · 一个 token · 无中继",
    family: "PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif",
    headlineSize: 74,
    leadSize: 25,
  },
};

for (const [file, card] of Object.entries(cards)) {
  const headline = card.lines
    .map((line, i) => `<text x="80" y="${292 + i * (card.headlineSize + 8)}" class="headline">${line}</text>`)
    .join("");
  const lead = card.lead
    .map((line, i) => `<text x="80" y="${472 + i * 34}" class="lead">${line}</text>`)
    .join("");
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <rect width="1200" height="630" fill="#f6f7f8"/>
  ${mark}
  <text x="154" y="112" class="wordmark">abacad</text>
  <text x="1120" y="112" text-anchor="end" class="eyebrow">${card.eyebrow}</text>
  ${headline}
  ${lead}
  <line x1="80" y1="556" x2="1120" y2="556" stroke="#e5e7ea" stroke-width="1"/>
  <text x="80" y="588" class="foot">abacad.ai</text>
  <text x="1120" y="588" text-anchor="end" class="foot">${card.foot}</text>
  <style>
    .wordmark { font: 600 30px ${card.family}; letter-spacing: -1px; fill: #15171a; }
    .eyebrow { font: 500 13px ui-monospace, Menlo, monospace; letter-spacing: 2.6px; fill: #63686f; }
    .headline { font: 600 ${card.headlineSize}px ${card.family}; letter-spacing: -3px; fill: #15171a; }
    .lead { font: 400 ${card.leadSize}px ${card.family}; fill: #63686f; }
    .foot { font: 500 13px ui-monospace, Menlo, monospace; letter-spacing: 2.2px; fill: #9aa0a7; }
  </style>
</svg>`;
  await sharp(Buffer.from(svg)).png().toFile(`public/${file}`);
  console.log(`wrote public/${file}`);
}
