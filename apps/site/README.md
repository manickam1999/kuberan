# Kuberan landing page

A static marketing page: plain HTML and CSS, one 20-line script for a "Copy" button, fonts and images
self-hosted. No framework, no build step, no analytics, no third-party requests. It lives apart from
`apps/web` on purpose, so a self-hosted Kuberan never serves marketing.

```
apps/site/
  index.html     the page
  style.css      Catppuccin Latte / Mocha tokens, light/dark by prefers-color-scheme
  site.js        copy-to-clipboard for the install commands (the page works without it)
  fonts/         Instrument Serif, Geist, Geist Mono (Latin subsets, SIL OFL 1.1)
  img/           WebP screenshots of the demo household, light and dark
  favicon.svg    the Polaris mark (solid, for small sizes)
  _headers       CSP and cache headers for Netlify / Cloudflare Pages
```

## Preview

Any static server works:

```bash
cd apps/site && python3 -m http.server 8000   # http://localhost:8000
```

## Before you publish

The demo link is a placeholder. It appears four times in `index.html`, always as
`https://demo.kuberan.example.com`. Replace it everywhere:

```bash
sed -i 's|https://demo.kuberan.example.com|https://demo.yourdomain.example|g' apps/site/index.html
```

The GitHub and docs links point at `https://github.com/zihangg/kuberan` (branch `main`); change them
the same way if you publish a fork. How to run the demo itself: [docs/public-demo.md](../../docs/public-demo.md).

## Deploy

Upload the contents of `apps/site/` to any static host. There is nothing to build.

- **Cloudflare Pages / Netlify:** set the publish directory to `apps/site` and leave the build command
  empty. `_headers` is picked up automatically.
- **GitHub Pages:** publish the `apps/site` folder (for example with the `actions/upload-pages-artifact`
  action, `path: apps/site`).
- **nginx, Caddy or your own VPS:** copy the folder to the web root, and send the headers from
  `_headers`. For example, in nginx:

  ```nginx
  add_header Content-Security-Policy "default-src 'none'; img-src 'self'; style-src 'self'; font-src 'self'; script-src 'self'; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'" always;
  add_header X-Content-Type-Options nosniff always;
  add_header Referrer-Policy no-referrer always;
  ```

The page also carries the same policy in a `<meta http-equiv>` tag (minus `frame-ancestors`, which only
works as a header), so it stays safe on hosts that cannot set headers.

## Screenshots

`img/` holds the only images. They are produced by `apps/web/scripts/screenshots.mjs` against a demo
instance (see [docs/screenshots.md](../../docs/screenshots.md)), then converted to WebP at quality 80:
the desktop shots at 1440x900, the phone shots at 390x844 captured at 2x. Refresh them whenever the
app's look changes; keep the file names, since `index.html` refers to them.

## Fonts

Instrument Serif, Geist and Geist Mono are licensed under the SIL Open Font License 1.1 and are the
Latin-subset files the web app already ships (extracted from its Next.js build).
