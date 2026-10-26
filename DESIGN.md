---
version: alpha
name: Cloudflare-dashboard-design-analysis
description: "A light, information-dense operations dashboard system built on a cool gray canvas (#f8fafc), pure white surfaces, and hairline borders (#e2e5e9). The signature Cloudflare orange (#f6821f) lives ONLY in the brand mark; all interactive color is a deep action blue (#0051c3) with soft blue focus rings. Typography is a single Inter-voice system at 400–600 with no display theatrics. Structure comes from a white 60px top bar with breadcrumbs, a white 232px left nav, and flat white cards with 1px borders — hierarchy by density and spacing, never by shadow. Status colors are muted semantic tints (soft green/amber/red backgrounds with dark readable text)."

colors:
  primary: "#0051c3"
  on-primary: "#ffffff"
  primary-hover: "#003d96"
  primary-active: "#00348b"
  focus-ring: "rgba(0, 81, 195, 0.15)"
  brand-orange: "#f6821f"
  brand-orange-light: "#fbad41"
  ink: "#1d1d1d"
  ink-muted: "#66707b"
  ink-subtle: "#8c9296"
  canvas: "#f8fafc"
  surface: "#ffffff"
  surface-hover: "#f1f4f7"
  surface-active: "#e8f1fb"
  hairline: "#e2e5e9"
  hairline-strong: "#c4cbd4"
  code-bg: "#f1f4f7"
  semantic-success: "#0a7d42"
  semantic-success-bg: "#e9f7ee"
  semantic-warning: "#9a6700"
  semantic-warning-bg: "#fdf3dc"
  semantic-danger: "#b33f31"
  semantic-danger-bg: "#fde8e7"
  banner-warning-border: "#ecd9a0"

typography:
  app-title:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.30
    letterSpacing: 0.2px
  card-title:
    fontFamily: Inter
    fontSize: 15px
    fontWeight: 600
    lineHeight: 1.30
    letterSpacing: 0
  section-label:
    fontFamily: Inter
    fontSize: 11.5px
    fontWeight: 600
    lineHeight: 1.30
    letterSpacing: 0.6px
  body:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: 0
  body-sm:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: 0
  caption:
    fontFamily: Inter
    fontSize: 12.5px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0
  micro:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: 400
    lineHeight: 1.40
    letterSpacing: 0
  button:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.20
    letterSpacing: 0
  button-sm:
    fontFamily: Inter
    fontSize: 12.5px
    fontWeight: 400
    lineHeight: 1.20
    letterSpacing: 0
  mono:
    fontFamily: "Consolas, ui-monospace, Menlo"
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0

rounded:
  xs: 3px
  sm: 4px
  md: 6px
  pill: 9999px

spacing:
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 28px
  xxl: 40px

## Overview

The Cloudflare dashboard system is a **light, dense operations canvas**. The page background is a cool gray `{colors.canvas}` (#f8fafc); every piece of chrome — top bar, left navigation, cards, tables — sits on pure white `{colors.surface}` separated from the canvas and from each other by 1px hairlines (`{colors.hairline}`). There is almost no shadow: elevation is expressed as stacking plus border, and only login/overlay surfaces earn a soft diffuse shadow.

Color is strictly rationed. **Brand orange** (`{colors.brand-orange}` #f6821f, with light accent `{colors.brand-orange-light}`) appears ONLY in the cloud logo mark. Every interactive element — primary buttons, links, active navigation, focus — uses the deep **action blue** `{colors.primary}` (#0051c3), hovering to `{colors.primary-hover}`. Focus is a soft blue ring (`{colors.focus-ring}`), never an outline swap. Semantic color exists only as status pills: dark readable text on a soft tinted background (success `{colors.semantic-success}` on `{colors.semantic-success-bg}`, warning, danger).

Type is one continuous Inter voice: `{typography.app-title}` at 600 for the wordmark, `{typography.card-title}` at 600 for card headings, `{typography.body}` 14px/400 for everything else, and `{typography.caption}`/`{typography.micro}` for meta. No display sizes exist — this is a working tool, not a marketing page. Negative tracking is never used; the only positive tracking is the uppercase `{typography.section-label}` nav group label.

Structure: a **white 60px sticky top bar** (logo + breadcrumb trail `管理后台 / <当前页>` left, account actions right), a **white 232px sticky left nav** (uppercase group label, then nav items that activate to `{colors.surface-active}` light blue with blue text), and a content column (max-width ~1000px) of white cards.

**Key Characteristics:**
- White-on-gray layering with 1px hairlines; shadows reserved for the login card and overlays.
- Orange = brand mark only. Blue = all interaction. Semantic tints = status only.
- Dense, tabular data presentation; row hover on `{colors.canvas}`-like tint.
- Controls are small-radius: 4px inputs/buttons, 6px cards, 3px status pills.
- Breadcrumb in the top bar anchors wayfinding instead of page titles.

## Colors

### Brand & Accent
- **Brand Orange** ({colors.brand-orange}): The Cloudflare cloud mark. Nowhere else.
- **Brand Orange Light** ({colors.brand-orange-light}): Two-tone highlight inside the mark.
- **Action Blue** ({colors.primary}): Primary buttons, links, active nav text, focus borders.
- **Action Blue Hover** ({colors.primary-hover}): Darker pressed/hovered state.
- **Focus Ring** ({colors.focus-ring}): 3px soft blue halo around focused inputs/buttons.

### Surface
- **Canvas** ({colors.canvas}): Page background behind everything.
- **Surface** ({colors.surface}): Top bar, sidebar, cards, inputs, buttons-secondary.
- **Surface Hover** ({colors.surface-hover}): Nav item hover, secondary button hover, code chips.
- **Surface Active** ({colors.surface-active}): Active nav item background.
- **Hairline** ({colors.hairline}): Card borders, dividers, table row rules.
- **Hairline Strong** ({colors.hairline-strong}): Input borders (darken to blue on focus).
- **Code BG** ({colors.code-bg}): Inline `<code>` chips.

### Text
- **Ink** ({colors.ink}): Primary text, near-black #1d1d1d.
- **Ink Muted** ({colors.ink-muted}): Labels, hints, table headers, nav sublabels.
- **Ink Subtle** ({colors.ink-subtle}): Placeholders, disabled text.

### Semantic
- **Success / Warning / Danger**: Muted dark text on soft tinted pill backgrounds; danger also outlines destructive buttons. Banner warning: `{colors.semantic-warning-bg}` with `{colors.banner-warning-border}` border.

## Typography

### Font Family

- **Inter** (with `system-ui, Segoe UI, Microsoft YaHei` fallback) — the single voice for all sizes. Chinese text renders through the CJK fallback at the same weights.
- **Mono** (Consolas stack) — IDs, hashes, tokens inside `<code>` chips only.

### Hierarchy

| Token | Size | Weight | Use |
|---|---|---|---|
| `{typography.app-title}` | 16px | 600 | Top bar wordmark |
| `{typography.card-title}` | 15px | 600 | Card headings, nav item labels |
| `{typography.section-label}` | 11.5px | 600 | Uppercase nav group label |
| `{typography.body}` | 14px | 400 | Default body, table cells |
| `{typography.body-sm}` | 13px | 400 | Detail tables, breadcrumbs |
| `{typography.caption}` | 12.5px | 400 | Hints, helper text |
| `{typography.micro}` | 11px | 400 | Nav sublabels |
| `{typography.button}` | 14px | 400 | Button labels |
| `{typography.button-sm}` | 12.5px | 400 | Ghost/compact buttons |
| `{typography.mono}` | 12px | 400 | Code chips |

### Principles

- No display scale. The largest weight-600 text is the 16px wordmark.
- Weight carries hierarchy (600 → 400); letter-spacing stays flat except the uppercase nav group label.
- Muted ink for labels-vs-values contrast inside detail tables.

## Layout

### Spacing System

4 / 8 / 12 / 16 / 24 / 28 / 40. Card padding 18–20px; content column padding 24–28px; card gap 16px.

### Grid & Container

- Top bar: full-width, 60px, sticky top, white, hairline bottom border.
- Sidebar: 232px, sticky under the top bar, white, hairline right border.
- Content: flexible column, `max-width: 1000px`; card grids `auto-fit, minmax(340px, 1fr)`.

### Whitespace Philosophy

Density over air. Rows are 28–34px tall; separation comes from hairlines, not padding gulfs. Cards stack with 16px gaps.

## Elevation & Depth

- **Level 0** (canvas): nothing.
- **Level 1** (cards/chrome): 1px hairline border + optional `0 1px 2px rgba(16,24,40,0.04)`.
- **Level 2** (login card): `0 4px 16px rgba(16,24,40,0.08)` — the only visible shadow in the product.
- Sticky top bar uses z-index, not shadow.

## Shapes

### Border Radius Scale

- `{rounded.sm}` 4px — buttons, inputs, checkboxes region.
- `{rounded.md}` 6px — cards, banner.
- `{rounded.xs}` 3px — status pills.
- Pills are NOT used for buttons. Cloudflare controls are square-ish.

## Components

### Buttons
- button-primary: `{colors.primary}` bg, white text, 1px same-color border, `{rounded.sm}`, 8px 16px, `{typography.button}`; hover `{colors.primary-hover}`; disabled 50% opacity.
- button-secondary (`ghost`): white bg, `{colors.hairline-strong}` border, `#3d4753` text, 5px 12px, `{typography.button-sm}`; hover `{colors.surface-hover}`.
- button-danger: white bg, 1px `{colors.semantic-danger}` border, danger text; hover `{colors.semantic-danger-bg}`.
- Focus on any button: blue border + `{colors.focus-ring}` halo.

### Inputs
- White bg, 1px `{colors.hairline-strong}` border, `{rounded.sm}`, 8px 10px.
- Focus: `{colors.primary}` border + `{colors.focus-ring}` halo. `color-scheme` follows theme.
- Labels: `{typography.caption}` muted, above the field; checkboxes inline with 16px accent-blue control.

### Cards
- White, 1px `{colors.hairline}`, `{rounded.md}`, level-1 shadow, 18–20px padding.
- Card head: title + action slot, 10px bottom padding, hairline divider.

### Status Pills
- 3px radius, 3px 10px padding, `{typography.caption}` weight 500.
- success/warning/danger = semantic text on semantic bg. No icons required.

### Detail Table
- Full-width, 13px; th muted 96px column; hairline row rules; last row borderless; row hover `#fafbfc`.

### Top Bar & Navigation
- Top bar: brand (orange mark + `{typography.app-title}` wordmark), hairline-separated breadcrumb `管理后台 / <页面>`, right-aligned secondary actions.
- Nav: uppercase `{typography.section-label}` group label; items stacked, 8px 10px padding, `{rounded.sm}`-on-hover; active = `{colors.surface-active}` bg + blue `{typography.card-title}` text. No left border bars.
- Login: centered 380px card (level-2 shadow) on canvas with a faint orange radial glow at top; brand mark above the title; single primary button full-width.

## Do's and Don'ts

### Do

- Reserve orange for the logo mark — everywhere else is blue.
- Use hairline borders + whitespace for hierarchy; reach for shadow only on overlays.
- Keep every status as muted-tint pill with dark readable text.
- Use breadcrumbs in the top bar for wayfinding.
- Keep controls compact: 4px radius, 32–36px height.
- Show feedback inline near the action (pills, form-ok/form-err text).

### Don't

- Don't introduce a dark theme or large low-contrast gray areas.
- Don't use orange as a button/link/focus color.
- Don't pill-round buttons or inputs.
- Don't add display-scale typography or letter-spacing games.
- Don't put icons where a text label fits; no decorative illustration.
- Don't use pure black (#000) text — ink is #1d1d1d.

## Responsive Behavior

| Name | Width | Key Changes |
|---|---|---|
| Desktop | ≥1280px | Sidebar + content side by side |
| Tablet | 720–1280px | Card grid wraps to 1–2 columns |
| Mobile | <720px | Sidebar stacks above content, full width; breadcrumb hidden; content padding 16px |

- Tap targets hold ≥36px height on touch.
- Card grids collapse `auto-fit` naturally; no separate mobile layout.

## Iteration Guide

1. Reference components by their `components:` token name when changing UI.
2. New interactive elements must state their blue/ghost/danger variant explicitly.
3. Any new color goes through semantic-tint review — the system only allows blue interaction + muted semantic pills.
4. Keep data displays tabular; new views should start from the detail-table pattern.
5. Run the full build (`cd web && npm run build`) after token changes and visually verify against this file.
