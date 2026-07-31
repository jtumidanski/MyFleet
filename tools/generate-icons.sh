#!/usr/bin/env bash
# tools/generate-icons.sh — regenerate the MyFleet icon set (FR-ICON-12).
#
# Outputs are COMMITTED. Neither CI nor apps/web/Dockerfile invokes this script
# or depends on any image-processing binary being installed; it exists so the
# assets can be reproduced when the mark changes.
#
# Backend preference — python3 + Pillow FIRST, deliberately:
#
#   Only the Python generator can emit favicon.svg and brandMarkPath.ts, because
#   the chevron geometry (not the SVG) is the single source. Preferring an SVG
#   rasteriser would mean that on a machine with ImageMagick installed, a
#   geometry change silently re-rasters a STALE favicon.svg while leaving the
#   SVG and the path constant stale too. A rasteriser is therefore only a
#   degraded fallback, and it says so.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLIC="$ROOT/apps/web/public"

if command -v python3 >/dev/null 2>&1 && python3 -c 'import PIL' >/dev/null 2>&1; then
    echo "generate-icons: backend = python3 + Pillow (full regeneration)"
    exec python3 "$ROOT/tools/generate-icons.py"
fi

for bin in rsvg-convert magick convert; do
    command -v "$bin" >/dev/null 2>&1 || continue

    echo "generate-icons: backend = $bin (DEGRADED — Pillow not found)" >&2
    echo "generate-icons: WARNING — favicon.svg, favicon.ico, icon-512-maskable.png and" >&2
    echo "generate-icons:           apps/web/src/components/brandMarkPath.ts are NOT" >&2
    echo "generate-icons:           regenerated on this path. If you changed the mark" >&2
    echo "generate-icons:           geometry, install Pillow and re-run." >&2

    if [ ! -f "$PUBLIC/favicon.svg" ]; then
        echo "generate-icons: ERROR — $PUBLIC/favicon.svg is missing and only Pillow can create it." >&2
        exit 1
    fi

    for spec in 180:apple-touch-icon 192:icon-192 512:icon-512; do
        size="${spec%%:*}"
        name="${spec##*:}"
        case "$bin" in
            rsvg-convert)
                rsvg-convert -w "$size" -h "$size" -b '#ffffff' \
                    "$PUBLIC/favicon.svg" -o "$PUBLIC/$name.png"
                ;;
            *)
                "$bin" -background '#ffffff' -flatten \
                    -resize "${size}x${size}" "$PUBLIC/favicon.svg" "$PUBLIC/$name.png"
                ;;
        esac
    done
    echo "generate-icons: done (partial)"
    exit 0
done

cat >&2 <<'EOF'
generate-icons: ERROR — no usable backend found.

Install ONE of the following and re-run:
  * python3 with Pillow   (preferred — regenerates every artefact)
        pip install Pillow
  * rsvg-convert          (librsvg; partial regeneration only)
  * ImageMagick           (`magick` or `convert`; partial regeneration only)
EOF
exit 1
