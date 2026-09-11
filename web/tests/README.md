# WebUI Smoke Test

Install test dependencies and run:

```bash
cd web
npm ci
npm run test:ui
```

The test starts a temporary static server, mocks the management APIs, switches through all five views, checks the Matrix drawer, settings tabs and node filters, and verifies 390, 768, 1024, and 1440px viewports for console errors and horizontal overflow.

If Chromium is not in a standard location, set:

```bash
PLAYWRIGHT_CHROMIUM_EXECUTABLE=/path/to/chrome npm run test:ui
```

Set `UI_SCREENSHOT_DIR` to save viewport screenshots:

```bash
UI_SCREENSHOT_DIR=/tmp/webui-screenshots npm run test:ui
```
