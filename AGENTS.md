# AGENTS.md

- Use TDD: write failing tests first, then implement.
- Keep the README up to date when adding features.
- The web dashboard (dashboard.html) must be responsive and work well on mobile browsers.
- Use Playwright to visually verify any front-end changes at both desktop (1280x800) and mobile (375x812) viewports. Save screenshots to the `tmp/` directory (e.g. `tmp/desktop.png`, `tmp/mobile.png`).
- Port 50233 is the default and is used by the production instance. Use a different port (e.g. --port 50234) when testing locally.
- When working with external APIs (Sensibo, PurpleAir, etc.), try hitting the actual API directly (e.g. via curl) to discover response formats rather than searching for documentation. The relevant API keys should be in the environment. If they don't appear to be set, ask the user.
- Any new UI setting or state (date range, selected view, filters, etc.) must be persisted in the URL query string so that reloading the page restores the same view.
- Visual design: all charts on the page must share the same x-axis (time) range so their time axes are visually aligned. Use `syncXAxes` / `computeXRange` from `src/axis-sync.js` after updating chart datasets.
- Visual design: variable y-axes (temperature `y`, PM2.5 `y2`) must also be aligned across all charts. Use `syncYAxes` / `computeYRange` from `src/axis-sync.js` for each axis id after setting datasets.
