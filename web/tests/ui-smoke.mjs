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
    body = { ok: true, installed: false, nodes: [], available_outbounds: [] };
  } else if (path.endsWith("/api/proxy/ports")) body = status.port_rules;
  else if (path.endsWith("/api/tunnel-groups")) body = [];
  else if (path.endsWith("/api/blacklist")) body = [];
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
    await page.evaluate(() => document.querySelector('[data-view="settings"]').click());
    await page.waitForTimeout(80);
    await page.locator('[data-action="switchSettingsTab"][data-args*="rotate"]').click();
    const rotateVisible = await page.locator("#tab-content-rotate").evaluate((element) => !element.classList.contains("hidden"));
    if (!rotateVisible) failures.push(`${viewport}px: settings tab switching failed`);
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
