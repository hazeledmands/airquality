# CLAUDE.md

- Use TDD: write failing tests first, then implement.
- Keep the README up to date when adding features.
- The web dashboard (dashboard.html) must be responsive and work well on mobile browsers.
- Use Playwright to visually verify any front-end changes at both desktop (1280x800) and mobile (375x812) viewports.
- Port 50233 is the default and is used by the production instance. Use a different port (e.g. --port 50234) when testing locally.
- When working with external APIs (Sensibo, PurpleAir, etc.), try hitting the actual API directly (e.g. via curl) to discover response formats rather than searching for documentation. The relevant API keys should be in the environment. If they don't appear to be set, ask the user.
