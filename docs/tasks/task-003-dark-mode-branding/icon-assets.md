# Icon & Branding Assets

Supporting spec for [`prd.md`](./prd.md) §4.8 (FR-ICON-1 … FR-ICON-12).

## Source of truth

One vector mark, authored once, is the source for every asset below. Constraints:

- Single colour, no gradients, no lettering.
- Geometric and legible when rendered at **16×16 CSS pixels** — test at that size before committing, not at 512.
- Authored on a 24×24 or 32×32 grid with even stroke weights so downscaling stays crisp.
- No text elements (they turn to mush at favicon sizes and complicate localisation).

The concrete shape is deliberately left to the design phase (see `prd.md` §9 open question 2).

## Asset matrix

All files live in `apps/web/public/` and are served from the site root.

| File | Size | Format | Purpose | Notes |
|---|---|---|---|---|
| `favicon.svg` | vector | SVG | Primary favicon | Embeds a `prefers-color-scheme` block so the mark stays legible against both light and dark browser chrome (FR-ICON-4) |
| `favicon.ico` | 16 + 32 | ICO | Fallback favicon | Multi-resolution. Declared as `rel="alternate icon"` so SVG-capable browsers prefer the vector (FR-ICON-5) |
| `apple-touch-icon.png` | 180×180 | PNG | iOS home screen | **Opaque background required.** iOS ignores transparency and composites onto black, which would erase a dark mark (FR-ICON-6) |
| `icon-192.png` | 192×192 | PNG | Manifest icon | `purpose: "any"` |
| `icon-512.png` | 512×512 | PNG | Manifest icon | `purpose: "any"` |
| `icon-512-maskable.png` | 512×512 | PNG | Manifest maskable icon | `purpose: "maskable"`. The mark must sit inside the safe zone — the inner 80% circle — because Android crops to arbitrary shapes |
| `site.webmanifest` | — | JSON | Web app manifest | See below |

Aggregate budget: **under 100 KB** (FR-PERF-4).

## `site.webmanifest`

```json
{
  "name": "MyFleet",
  "short_name": "MyFleet",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#ffffff",
  "theme_color": "#ffffff",
  "icons": [
    { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any" },
    { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any" },
    { "src": "/icon-512-maskable.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable" }
  ]
}
```

The manifest's `theme_color` is static — the manifest format has no media-query support. Per-theme browser chrome comes from the two `<meta name="theme-color">` tags instead, which do support `media`.

## `index.html` `<head>` additions

Added to `apps/web/index.html` alongside the pre-paint theme script (FR-FLASH-1):

```html
<link rel="icon" href="/favicon.svg" type="image/svg+xml" />
<link rel="alternate icon" href="/favicon.ico" sizes="16x16 32x32" />
<link rel="apple-touch-icon" href="/apple-touch-icon.png" />
<link rel="manifest" href="/site.webmanifest" />
<meta name="theme-color" media="(prefers-color-scheme: light)" content="#ffffff" />
<meta name="theme-color" media="(prefers-color-scheme: dark)" content="#020817" />
```

The two `theme-color` values are the rendered equivalents of the existing `--background` tokens in `apps/web/src/index.css`: `0 0% 100%` → `#ffffff`, `222.2 84% 4.9%` → `#020817`. If those tokens ever change, these must change with them.

## In-app mark

`apps/web/src/components/BrandMark.tsx` renders the same geometry inline as SVG:

- Uses `fill="currentColor"` (or `stroke="currentColor"`) so it inherits the surrounding text colour and needs no dark variant.
- Accepts a `className` for sizing; no hardcoded pixel dimensions in the component.
- `aria-hidden="true"` where it sits beside the visible "MyFleet" wordmark in `AppLayout` (FR-ICON-10) — it is decorative there, and a duplicate accessible name would make screen readers announce the brand twice.

The inline component and `favicon.svg` share geometry but are separate files. Keeping the favicon a static asset avoids pulling SVG-loader plugins into the Vite config for one icon.

## Generation

`tools/generate-icons.sh` documents and reproduces PNG/ICO rasterisation from `favicon.svg`.

- Outputs are **committed to the repository** (FR-ICON-12).
- Neither CI nor `apps/web/Dockerfile` may invoke the script or depend on any image-processing binary.
- The script should detect its available backend (`rsvg-convert`, ImageMagick `magick`/`convert`, or `npx sharp-cli`) and fail with a clear message naming the alternatives if none is present.
- It is a maintenance convenience, not part of the build graph.

## Delivery path (no build changes required)

Vite copies `apps/web/public/**` verbatim into `apps/web/dist/`. The existing Dockerfile already does:

```dockerfile
COPY apps/web ./apps/web            # picks up public/
RUN npm run -w @myfleet/web build   # public/ → dist/
COPY --from=build /app/apps/web/dist /usr/share/nginx/html
```

and `apps/web/nginx.conf` serves `root /usr/share/nginx/html` with `try_files $uri $uri/ /index.html`, so each asset resolves as a real file before the SPA fallback. No Dockerfile, nginx, or Vite configuration change is needed (FR-ICON-2).

## Verification

- [ ] Tab icon renders in Chrome, Firefox, and Safari.
- [ ] `favicon.svg` legible against both light and dark browser chrome.
- [ ] iOS "Add to Home Screen" shows the mark on an opaque background.
- [ ] Manifest validates; app is installable with the correct name and icons.
- [ ] Maskable icon survives circular and squircle cropping without clipping the mark.
- [ ] Mark renders identically in the sidebar and adapts to theme via `currentColor`.
- [ ] Total asset size under 100 KB.
- [ ] In the built container, `/favicon.svg`, `/favicon.ico`, `/apple-touch-icon.png`, and `/site.webmanifest` each return 200 (not the SPA `index.html` fallback).
