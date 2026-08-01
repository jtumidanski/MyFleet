# Status token contrast record

FR-A11Y-1 requires a recorded ratio for every new token pairing in both themes.
These are **computed**, not estimated — reproduce them with the script at the
bottom of this file.

`--background` and `--card` hold identical values in both themes (`0 0% 100%`
light, `222.2 84% 4.9%` dark), so "bare on background" and "bare on card"
collapse into one measurement per family per theme. That gives the sixteen
pairings the design calls for: 4 families × 2 measurements × 2 themes.

## Light theme

| Family | bare on `--background` / `--card` | `-subtle-foreground` on `-subtle` |
|---|---|---|
| success | 5.02:1 | 6.50:1 |
| warning | 5.01:1 | 6.36:1 |
| danger | 6.67:1 | 6.80:1 |
| info | 6.71:1 | 7.15:1 |

## Dark theme

| Family | bare on `--background` / `--card` | `-subtle-foreground` on `-subtle` |
|---|---|---|
| success | 14.23:1 | 10.25:1 |
| warning | 11.99:1 | 10.97:1 |
| danger | 7.23:1 | 8.14:1 |
| info | 7.85:1 | 8.60:1 |

**All sixteen clear the 4.5:1 body-text threshold.** The tightest is light
`--warning` on white at 5.01:1.

## MaintenanceQueueView overdue row

The overdue row in `MaintenanceQueueView.tsx` fills with `bg-danger-subtle`.
Body text inside that row must therefore be measured against `--danger-subtle`,
not against `--background` — the "danger" row above already gives the accepted
pairing, restated here explicitly because it governs a specific component:

| Pairing | Light | Dark |
|---|---|---|
| `danger-subtle-foreground` on `danger-subtle` (accepted, used for "Was due …" / "At … miles") | 6.80:1 | 8.14:1 |
| `muted-foreground` on `danger-subtle` (rejected — do not reintroduce) | 3.89:1 **FAIL** | 6.38:1 |

`text-muted-foreground` is calibrated against `--background`/`--card`, not
against the danger-subtle fill, so it silently drops below AA the moment it
lands inside a `bg-danger-subtle` container even though it passes everywhere
else in the app. `text-danger-subtle-foreground` is the correct token for any
body text inside that fill.

## On the `-border` tokens

The `-border` values are deliberately low-contrast against the page background
(1.2–2.3:1) and are **not** part of the FR-A11Y-1 requirement. They are
decorative separators around chips and callouts whose meaning is carried by a
text label in every case (FR-A11Y-2) — they are not the sole means of
identifying a UI component or its state, which is what WCAG 1.4.11 governs. A
chip border pulled to 3:1 would read as a heavy outline and fight the subtle
fill it encloses.

## Reproducing

```python
import colorsys

def rgb(h, s, l):
    return colorsys.hls_to_rgb(h / 360.0, l / 100.0, s / 100.0)

def luminance(c):
    f = lambda v: v / 12.92 if v <= 0.03928 else ((v + 0.055) / 1.055) ** 2.4
    r, g, b = (f(v) for v in c)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b

def ratio(a, b):
    la, lb = luminance(rgb(*a)), luminance(rgb(*b))
    hi, lo = max(la, lb), min(la, lb)
    return (hi + 0.05) / (lo + 0.05)

LIGHT = {
    'success': {'bare': (142.4, 71.8, 29.2), 'subtle': (140.6, 84.2, 92.5), 'sf': (142.8, 64.2, 24.1)},
    'warning': {'bare': (26, 90.5, 37.1), 'subtle': (48, 96.5, 88.8), 'sf': (22.7, 82.5, 31.4)},
    'danger':  {'bare': (0, 72.2, 41.1), 'subtle': (0, 93.3, 94.1), 'sf': (0, 70, 35.3)},
    'info':    {'bare': (224.3, 76.3, 48), 'subtle': (214.3, 94.6, 92.7), 'sf': (226, 70.7, 40.2)},
}
DARK = {
    'success': {'bare': (141.7, 76.6, 73.1), 'subtle': (142, 40, 14), 'sf': (142, 70, 78)},
    'warning': {'bare': (43.3, 96.4, 56.3), 'subtle': (30, 45, 14), 'sf': (43, 90, 76)},
    'danger':  {'bare': (0, 90.6, 70.8), 'subtle': (0, 45, 15), 'sf': (0, 90, 80)},
    'info':    {'bare': (213.1, 93.9, 67.8), 'subtle': (217, 45, 17), 'sf': (213, 92, 80)},
}

for theme, table, bg in (('light', LIGHT, (0, 0, 100)), ('dark', DARK, (222.2, 84, 4.9))):
    for family, t in table.items():
        print(f"{theme:6} {family:8} bare-on-bg {ratio(t['bare'], bg):5.2f}:1"
              f"   sf-on-subtle {ratio(t['sf'], t['subtle']):5.2f}:1")
```
