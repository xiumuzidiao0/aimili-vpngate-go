import { createServer } from "node:http";
import { existsSync, readFileSync, mkdirSync, statSync } from "node:fs";
import { extname, join, resolve } from "node:path";
import { chromium } from "playwright-core";

const root = resolve(new URL("../dist/", import.meta.url).pathname);
const screenshotDir = process.env.UI_SCREENSHOT_DIR || "";
const mimeTypes = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".svg": "image/svg+xml",
  ".ttf": "font/ttf",
};

function findChromium() {
  const candidates = [
    process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
    "/home/xmzd/.cache/ms-playwright/chromium-1234/chrome-linux/chrome",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
    "/usr/bin/google-chrome",
  ].filter(Boolean);
  return candidates.find((path) => existsSync(path));
}

function startServer() {
  const server = createServer((req, res) => {
    const pathname = decodeURIComponent(new URL(req.url, "http://127.0.0.1").pathname);
    const filePath = join(root, pathname === "/" ? "index.html" : pathname);
    if (!filePath.startsWith(root) || !existsSync(filePath) || !statSync(filePath).isFile()) {
      res.writeHead(404);
      res.end("not found");
      return;
    }
    res.writeHead(200, { "Content-Type": mimeTypes[extname(filePath)] || "application/octet-stream" });
    res.end(readFileSync(filePath));
  });
  return new Promise((resolveServer) => {
    server.listen(0, "127.0.0.1", () => resolveServer(server));
  });
}

const status = {
  vpn: {
    status: "connected",
    status_text: "已连接",
    active_node_id: "jp-1",
    active_node: {
      id: "jp-1",
      ip: "203.0.113.42",
      port: 1194,
      country_short: "JP",
      ip_type: "residential",
      isp: "Example Fiber",
    },
    uptime_seconds: 4820,
    last_message: "主隧道运行正常",
  },
  traffic: {
    download_speed_bps: 8320000,
    upload_speed_bps: 1240000,
    total_download_bytes: 5823000000,
    total_upload_bytes: 822000000,
    active_connections: 37,
  },
  proxy_addr: "127.0.0.1:7928",
  node_count: 1,
  total_node_count: 1,
  node_source: "Fastly",
  blacklist_count: 0,
  version: "2.5.0",
  tunnels: [],
  port_rules: [{ port: 7928, enabled: true, auth_mode: "default_web", policy: "round_robin" }],
  dynamic_groups: [],
};

const nodes = [{
  id: "jp-1",
  ip: "203.0.113.42",
  port: 1194,
  country_short: "JP",
  country_long: "日本",
  region: "Tokyo",
  city: "Tokyo",
  hostname: "jp.example.test",
  proto: "udp",
  score: 87420,
  speed: 48300000,
  latency_ms: 42,
  ping: 18,
  reputation_score: 91,
  is_favorite: true,
  ip_type: "residential",
  isp: "Example Fiber",
  unlock: { openai: "unlocked", claude: "unlocked", netflix: "unlocked", google: "unlocked" },
}];

let lastOutboundPost = null;

async function mockAPI(route) {
  const path = new URL(route.request().url()).pathname;
  if (path.endsWith("/api/events")) {
    await route.fulfill({ status: 200, contentType: "text/event-stream", body: ": connected\n\n" });
    return;
  }
  let body = { ok: true };
  if (path.endsWith("/api/status")) body = status;
  else if (path.endsWith("/api/nodes")) body = nodes;
  else if (path.endsWith("/api/unlock")) body = {};
  else if (path.endsWith("/api/singbox/overview")) {
    body = {
      ok: true,
      installed: true,
      nodes: [{ name: "Hysteria2-62799.json", protocol: "Hysteria2", port: 62799, outbound: "direct" }],
      available_outbounds: [
        { addr: "direct", label: "直连", is_default: false },
        { addr: "http://127.0.0.1:7928", label: "默认出口 7928", is_default: true }
      ]
    };
  } else if (path.endsWith("/api/singbox/nodes/outbound")) {
    try {
      lastOutboundPost = JSON.parse(route.request().postData() || "{}");
    } catch {}
    body = { ok: true };
  } else if (path.endsWith("/api/proxy/ports")) {
    body = status.port_rules;
  } else if (path.endsWith("/api/tunnel-groups")) {
    body = [
      {
        id: "dg-1",
        name: "日本Top3住宅组",
        enabled: true,
        country: "JP",
        ip_type: "residential",
        sort_by: "latency",
        unlock_filter: "ai",
        target_count: 3,
        interval_minutes: 15,
        active_tunnel_ids: ["t-1"],
        status_text: "正常"
      }
    ];
  } else if (path.endsWith("/api/settings")) {
    body = {
      ui_port: 8787,
      ui_path: "aimili",
      ui_username: "admin",
      proxy_port: 7928,
      auto_rotate_minutes: 15,
      auto_rotate_ip_type: "residential",
      discovery_countries: ["JP", "US"],
      telegram_bot_token: "123456:ABC-DEF",
      telegram_chat_id: "987654321",
    };
  } else if (path.endsWith("/api/blacklist")) {
    body = [];
  }
  await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
}

const server = await startServer();
const address = server.address();
const baseURL = `http://127.0.0.1:${address.port}`;
const executablePath = findChromium();
if (!executablePath) {
  throw new Error("Chromium executable not found; set PLAYWRIGHT_CHROMIUM_EXECUTABLE");
}
if (screenshotDir) mkdirSync(screenshotDir, { recursive: true });

const browser = await chromium.launch({ executablePath, headless: true, args: ["--no-sandbox"] });
const failures = [];
try {
  for (const viewport of [390, 768, 1024, 1440]) {
    const page = await browser.newPage({ viewport: { width: viewport, height: 900 } });
    const errors = [];
    page.on("console", (message) => {
      if (message.type() === "error") errors.push(message.text());
    });
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("**/api/**", mockAPI);
    await page.goto(`${baseURL}/#matrix`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(500);
    await page.locator('[data-action="showAddPortForm"]').click();
    await page.waitForTimeout(250);
    const drawerOpen = await page.locator("#port-edit-card").evaluate((element) => element.classList.contains("open"));
    await page.locator("#port-edit-card [data-drawer-close]").click();
    await page.waitForTimeout(260);
    for (const view of ["dashboard", "singbox", "matrix", "settings", "nodes"]) {
      await page.evaluate((target) => {
        document.querySelector(`[data-view="${target}"]`).click();
      }, view);
      await page.waitForTimeout(80);
      const active = await page.locator(`#view-${view}`).evaluate((element) => element.classList.contains("active"));
      if (!active) failures.push(`${viewport}px: view ${view} did not activate`);
    }
    // Test Settings tabs visibility
    await page.evaluate(() => document.querySelector('[data-view="settings"]').click());
    await page.waitForTimeout(100);
    const baseVisible = await page.locator("#tab-content-base").isVisible();
    if (!baseVisible) failures.push(`${viewport}px: settings base tab is not visible`);

    await page.locator('[data-action="switchSettingsTab"][data-args*="rotate"]').click();
    await page.waitForTimeout(60);
    const rotateVisible = await page.locator("#tab-content-rotate").isVisible();
    if (!rotateVisible) failures.push(`${viewport}px: settings rotate tab is not visible`);

    await page.locator('[data-action="switchSettingsTab"][data-args*="tg"]').click();
    await page.waitForTimeout(60);
    const tgVisible = await page.locator("#tab-content-tg").isVisible();
    if (!tgVisible) failures.push(`${viewport}px: settings tg tab is not visible`);

    // Test Matrix & Dynamic Groups visibility
    await page.evaluate(() => document.querySelector('[data-view="matrix"]').click());
    await page.waitForTimeout(100);
    const portsVisible = await page.locator("#matrix-content-ports").isVisible();
    if (!portsVisible) failures.push(`${viewport}px: matrix ports content is not visible`);

    await page.locator('[data-action="switchMatrixTab"][data-args*="groups"]').click();
    await page.waitForTimeout(60);
    const groupsVisible = await page.locator("#matrix-content-groups").isVisible();
    if (!groupsVisible) failures.push(`${viewport}px: matrix dynamic groups content is not visible`);
    const groupCardVisible = await page.locator(".dynamic-group-card").first().isVisible();
    if (!groupCardVisible) failures.push(`${viewport}px: dynamic group card is not visible in matrix tab`);

    // Test sing-box outbound select switching and add modal
    lastOutboundPost = null;
    await page.evaluate(() => document.querySelector('[data-view="singbox"]').click());
    await page.waitForTimeout(100);

    const btnText = await page.locator("#btn-add-sb-node").innerText();
    if (btnText.trim().startsWith("+")) {
      failures.push(`${viewport}px: singbox add button still has duplicate plus sign: "${btnText}"`);
    }

    await page.locator("#btn-add-sb-node").click();
    await page.waitForTimeout(100);
    const modalOpen = await page.locator("#singbox-add-modal").isVisible();
    if (!modalOpen) failures.push(`${viewport}px: singbox add modal did not open`);

    const totalCards = await page.locator("#singbox-add-modal .choice-card").count();
    if (totalCards !== 22) failures.push(`${viewport}px: expected 22 protocol cards, got ${totalCards}`);

    await page.locator('[data-action="filterProtoGrid"][data-args*="recommended"]').click();
    await page.waitForTimeout(60);
    const visibleRec = await page.locator("#singbox-add-modal .choice-card:visible").count();
    if (visibleRec !== 5) failures.push(`${viewport}px: expected 5 recommended cards, got ${visibleRec}`);

    await page.locator('#proto-card-rh2').click();
    const rh2Selected = await page.locator('#proto-card-rh2').evaluate(el => el.classList.contains('selected'));
    if (!rh2Selected) failures.push(`${viewport}px: rh2 protocol card was not selected`);

    await page.locator('[data-action="closeAddSingBoxModal"]').first().click();
    await page.waitForTimeout(100);

    const select = page.locator('.sb-chain-select');
    if (await select.count() > 0) {
      await select.selectOption("http://127.0.0.1:7928");
      await page.waitForTimeout(100);
      if (!lastOutboundPost || lastOutboundPost.outbound !== "http://127.0.0.1:7928" || lastOutboundPost.target !== "Hysteria2-62799.json") {
        failures.push(`${viewport}px: singbox outbound select did not trigger POST /api/singbox/nodes/outbound with expected payload, got: ${JSON.stringify(lastOutboundPost)}`);
      }
    } else {
      failures.push(`${viewport}px: singbox chain select element not found`);
    }

    await page.locator('[data-view="nodes"]:visible').click();
    await page.locator("#chip-fav").click();
    const metrics = await page.evaluate(() => ({
      overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
      inlineHandlers: [...document.querySelectorAll("*")].filter((element) =>
        [...element.attributes].some((attribute) => attribute.name.startsWith("on"))
      ).length,
      favoriteFilter: document.querySelector("#chip-fav")?.classList.contains("active"),
      fontReady: document.fonts.check("14px Geist"),
    }));
    if (!drawerOpen) failures.push(`${viewport}px: editor drawer did not open`);
    if (metrics.overflow) failures.push(`${viewport}px: horizontal overflow`);
    if (metrics.inlineHandlers !== 0) failures.push(`${viewport}px: inline handlers remain`);
    if (!metrics.favoriteFilter) failures.push(`${viewport}px: filter action failed`);
    if (!metrics.fontReady) failures.push(`${viewport}px: Geist font did not load`);
    if (errors.length) failures.push(`${viewport}px console: ${errors.join("; ")}`);
    if (screenshotDir) {
      await page.screenshot({ path: join(screenshotDir, `webui-${viewport}.png`), fullPage: true });
    }
    await page.close();
  }
} finally {
  await browser.close();
  await new Promise((resolveClose) => server.close(resolveClose));
}

if (failures.length) {
  console.error(failures.join("\n"));
  process.exit(1);
}
console.log("WebUI smoke test passed for 390, 768, 1024 and 1440px viewports.");
