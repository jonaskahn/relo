---
name: color
description: Theme, accent and semantic colours; accent scale derivation; contrast rules.
requires: []
---

# Color

One accent drives every tint. The operator picks it at runtime (`data-accent` on `<html>`);
components only ever read tokens, never hex values.

## Theme tokens (`console/src/app.css`)

```css
:root{
  color-scheme:light;
  --canvas:#f5f5f7; --surface:#fff; --raised:#fff; --sunken:#f5f5f7;
  --ink:#17171c; --muted:#6b6b78; --faint:#858592;   /* faint: icons/decoration only */
  --line:rgb(0 0 0/.09); --line-strong:rgb(0 0 0/.16);
  --hover:color-mix(in srgb,var(--ink) 4%,var(--surface));
  --shadow:rgb(20 20 40/.10);
  --ok:#19774e; --warn:#9b5800; --danger:#bd342a;     /* fixed semantics, never follow accent */
  --chrome:rgb(245 245 247/.82);
  --shade:#000; --mixbase:var(--surface); --t50:7%; --t100:14%; --t200:26%;
  --accent:var(--accent-base);
}
:root[data-theme="dark"]{
  color-scheme:dark;
  --canvas:#0a0a0a; --surface:#111113; --raised:#1a1a1d; --sunken:#0a0a0a;
  --ink:#f2f2f5; --muted:#a3a3b0; --faint:#6f6f7c;
  --line:rgb(255 255 255/.08); --line-strong:rgb(255 255 255/.16);
  --shadow:rgb(0 0 0/.55);
  --ok:#4cc38a; --warn:#f0a93b; --danger:#ff7468;
  --chrome:rgb(10 10 10/.72);
  --shade:#fff; --t50:12%; --t100:22%; --t200:34%;
  --accent:color-mix(in oklab,var(--accent-base) 72%,#fff);
  --on-accent:#0a0a0a;                                /* every accent, dark theme */
}
```

## Accent scale (derived, keep the existing names)

```css
:root{
  --accent-50: color-mix(in oklab,var(--accent) var(--t50), var(--mixbase));
  --accent-100:color-mix(in oklab,var(--accent) var(--t100),var(--mixbase));
  --accent-200:color-mix(in oklab,var(--accent) var(--t200),var(--mixbase));
  --accent-300:color-mix(in oklab,var(--accent) 44%,var(--mixbase));
  --accent-400:color-mix(in oklab,var(--accent) 70%,var(--mixbase));
  --accent-500:var(--accent);
  --accent-600:color-mix(in oklab,var(--accent) 86%,var(--shade));
  --accent-700:color-mix(in oklab,var(--accent) 72%,var(--shade));
  --accent-800:color-mix(in oklab,var(--accent) 56%,var(--shade));
  --accent-soft:var(--accent-50);  --accent-ink:var(--accent-800);
  --accent-border:color-mix(in oklab,var(--accent) 32%,var(--mixbase));
  --accent-glow:color-mix(in srgb,var(--accent) 30%,transparent);
}
```

| Use                             | Token                                                                                  |
|---------------------------------|----------------------------------------------------------------------------------------|
| Filled control background       | `--accent` (text `--on-accent`)                                                        |
| Selected row/chip/tab fill      | `--accent-50` + border `--accent-border` + text `--accent-ink`                         |
| Focus ring                      | `--accent` 2px, halo `--accent-100` 3px on inputs                                      |
| Accent text on surface          | `--accent-700`, or `--accent-ink` on a soft fill. Never `--accent` for text under 18px |
| Bar fill / switch on / rank dot | `--accent`                                                                             |

## Accents (`data-accent`)

`:root` default is `red`. Bases other than red were sampled from the picker.

| id            | `--accent-base` | light `--on-accent` |
|---------------|-----------------|---------------------|
| red (default) | `#c8102e`       | `#fff`              |
| orange        | `#d9622b`       | `#17171c`           |
| amber         | `#cc7c2e`       | `#17171c`           |
| green         | `#43946c`       | `#17171c`           |
| teal          | `#439288`       | `#17171c`           |
| cyan          | `#418fae`       | `#17171c`           |
| blue          | `#3662e3`       | `#fff`              |
| indigo        | `#4e46dc`       | `#fff`              |
| purple        | `#743ee4`       | `#fff`              |
| rose          | `#cf364c`       | `#fff`              |

```css
[data-accent="orange"]{--accent-base:#d9622b;--on-accent:#17171c}  /* one block per row */
```

Light on-accent was chosen for 4.5:1 or better (white on orange/amber/green/teal/cyan is only 3.2–3.7). Dark theme lifts
every accent and uses near-black text (5.7:1 or better).

## Semantic colour

- `--ok`, `--warn`, `--danger` are fixed and independent of the accent. Tints:
  `color-mix(in srgb,var(--danger) 12%,var(--surface))`.
- Destructive actions and errors always use `--danger`.
- Status is never colour alone: pair with text, and with an icon for warn and danger.
- Neutral text: `--ink` 17.9:1, `--muted` 5.2:1 on white (4.8:1 on canvas). `--faint` is below 4.5:1: never for text
  that carries meaning.

## Don't

- Hard-code hex, `rgb()` or Tailwind palette colours in components. Logos and chart data are the only exceptions.
- Tint backgrounds with `opacity`; use the `--accent-*` or `color-mix` tokens so dark theme works.
- Use the accent to mean "bad". Warning and error have their own tokens.
