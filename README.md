# quilzo.github.io

Public mirror of the Quilzo manual — the same documentation shipped inside
the app itself (Help → Documentation), published here so it's reachable
without an account.

Live at: https://quilzo.github.io

## What's here

- `index.html` / `style.css` / `app.js` — the site (a single static page)
- `images/` — the 8 screenshots referenced by the manual
- `.nojekyll` — tells GitHub Pages to serve these files as-is

There's no build step or framework. The content was extracted directly from
`internal/admin/docs_*.go` in the main [Quilzo repo](https://github.com/Quilzo/Quilzo)
(the structured `chapter`/`section`/`block` data that powers the in-app
manual), so the wording here matches the app exactly rather than being
maintained separately.
