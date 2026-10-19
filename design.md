# Design - ChasteDesk

Locked design system. Future UI work reads this file first and defers to it. Amend it intentionally; the file is the rule.

## System

- Genre: modern-minimal with an editorial-utilitarian voice
- Macrostructure: Workbench for product pages, Split Studio for auth and invitation pages
- Theme: custom, "Warm Ledger"
- Axes: warm paper band / Instrument Sans body with Instrument Serif display / persimmon signal accent
- Mood: a calm, precise support desk. Warm paper, dark spruce navigation, one persimmon accent. Not playful, not sterile.

## Tokens

Canonical values live in `internal/web/static/css/tokens.css`. Do not inline raw colors in page CSS.

```css
:root {
  --color-paper:      oklch(97.7% 0.011 82);
  --color-surface:    oklch(99.3% 0.005 82);
  --color-ink:        oklch(22% 0.016 60);
  --color-muted:      oklch(47% 0.012 66);
  --color-rule:       oklch(89% 0.011 78);
  --color-accent:     oklch(61% 0.17 42);
  --color-focus:      oklch(58% 0.19 40);
  --color-sidebar:    oklch(21% 0.018 158);

  --font-display: "Instrument Serif", ui-serif, Georgia, serif;
  --font-body:    "Instrument Sans", ui-sans-serif, system-ui, sans-serif;
  --font-mono:    "JetBrains Mono", ui-monospace, monospace;

  --radius-sm: 0.375rem;
  --radius-md: 0.625rem;
  --radius-lg: 0.875rem;
  --radius-pill: 999px;

  --ease-out: cubic-bezier(0.16, 1, 0.3, 1);
  --dur-fast: 140ms;
  --dur-base: 220ms;
  --dur-slow: 320ms;
}
```

The dark theme keeps the same hue anchors and moves lightness only. Both `[data-theme="dark"]` and `prefers-color-scheme` are supported.

## Type

- Display: Instrument Serif 400 for page titles, wordmark, and empty-state titles. No italic headers.
- Body: Instrument Sans 400 to 650. UI labels use 600 at small sizes.
- Mono: JetBrains Mono for metadata, IDs, timestamps, counts, and code.
- Scale: major third, 1.25, starting from a 15px UI base.
- Numbers: `font-variant-numeric: tabular-nums` on all data displays.

## CTA voice

- Primary: ink fill, paper text, 6px radius, 40px height. Used for the one next action on a screen.
- Secondary: surface fill with a strong hairline. Used for tests, filters, and adjacent actions.
- Ghost: transparent, used for skip, cancel, and tertiary navigation.
- Danger: transparent with a red rule and red text. Never a solid red button.
- Accent fill is reserved for the launcher, active navigation marker, and status dots. It never covers large surfaces.

## Motion

- Motion-cut by default. One page entrance, button press feedback, and copy confirmation.
- Popovers and dialogs use opacity plus a small translate, never a bounce.
- Reduced motion collapses all spatial motion to an instant opacity change.
- Polling swaps must not restart page entrances.

## Layout

- 12-column content grid with full-width tables and lists where density matters.
- Sidebar is a fixed 16.75rem spruce rail on desktop, an off-canvas sheet below 60rem.
- Content max width is 92rem with fluid gutters.
- Cards use one hairline border and a whisper shadow. No nested cards.
- Forms use 44px controls, labels above inputs, reserved helper height, and no border-width changes on focus.
- Every clickable label is single-line. Rows wrap, labels do not.

## Icons

One inline stroke set at 1.75 weight, 24px viewBox, rendered at 1rem by default. No mixed icon libraries and no emoji as interface icons.

## Exports

`internal/web/static/css/tokens.css` is the source of truth for CSS variables. For a Tailwind v4 `@theme`, DTCG `tokens.json`, or shadcn/ui variable set, translate from that file rather than maintaining a second palette.

## Notes

- No gradients, glassmorphism, aurora blobs, or decorative 3D.
- No invented metrics in the UI. Every number comes from the database.
- No third-party fonts or scripts. Everything is self-hosted.
- No em dashes in interface copy, documentation, or code comments.
