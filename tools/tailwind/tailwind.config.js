// Tailwind, compiled (audit 4.3) instead of the Play CDN, which shipped a
// 400 KB compiler that scanned the page on every mutation. The theme is the one
// that was inline in index.html; the content list is every file that carries
// class names, Alpine's :class objects included. Build with ./build.sh.
module.exports = {
    darkMode: 'class',
    // hover: classes only where a pointer hovers: an iPhone kept a tapped
    // element in its hover style (29/10).
    future: { hoverOnlyWhenSupported: true },
    // Every script under www/ but the vendored libraries: a folder added later
    // (www/store/ on 12/10 went unlisted for a day, and eight classes of the
    // aircraft sheet went uncompiled) is scanned without anyone remembering to.
    content: [
        '../../www/index.html',
        '../../www/**/*.js',
        '!../../www/vendor/**',
    ],
    theme: {
        extend: {
            colors: {
                // The eight names the page has used since upstream, on the same
                // values as always (migration step 1: identical behaviour; their
                // opacity modifiers, bg-panel/50, keep working on a hex).
                background: '#0a0a0a',
                panel: '#1a1a1a',
                text: '#e0e0e0',
                highlight: '#4CAF50',
                warning: '#FFC107',
                danger: '#F44336',
                border: '#333',
                hover: '#2a2a2a',
                // The design system's tokens (www/tokens.css, from jetons.json),
                // reachable as utilities: bg-surface-panel, text-text-secondary,
                // border-border-subtle, text-status-urgent... No opacity modifier
                // on these (they are variables, not channels): use the token.
                surface: {
                    map: 'var(--color-surface-map)',
                    page: 'var(--color-surface-page)',
                    panel: 'var(--color-surface-panel)',
                    raised: 'var(--color-surface-raised)',
                    hover: 'var(--color-surface-hover)',
                    selected: 'var(--color-surface-selected)',
                    urgent: 'var(--color-surface-urgent)',
                    warn: 'var(--color-surface-warn)',
                },
                'text-primary': 'var(--color-text-primary)',
                'text-secondary': 'var(--color-text-secondary)',
                'text-tertiary': 'var(--color-text-tertiary)',
                'text-disabled': 'var(--color-text-disabled)',
                'on-accent': 'var(--color-text-on-accent)',
                'on-urgent': 'var(--color-text-on-urgent)',
                'on-warn': 'var(--color-text-on-warn)',
                'border-subtle': 'var(--color-border-subtle)',
                'border-default': 'var(--color-border-default)',
                'border-control': 'var(--color-border-control)',
                'border-focus': 'var(--color-border-focus)',
                accent: {
                    DEFAULT: 'var(--color-accent-default)',
                    strong: 'var(--color-accent-strong)',
                    fill: 'var(--color-accent-fill)',
                },
                status: {
                    urgent: 'var(--color-status-urgent)',
                    'urgent-fill': 'var(--color-status-urgent-fill)',
                    warn: 'var(--color-status-warn)',
                    info: 'var(--color-status-info)',
                    ok: 'var(--color-status-ok)',
                    unknown: 'var(--color-status-unknown)',
                },
                phase: {
                    ground: 'var(--color-phase-ground)',
                    departure: 'var(--color-phase-departure)',
                    cruise: 'var(--color-phase-cruise)',
                    arrival: 'var(--color-phase-arrival)',
                    unknown: 'var(--color-phase-unknown)',
                },
            },
            fontSize: {
                caption: ['var(--font-size-caption)', 'var(--font-line-height-caption)'],
                small: ['var(--font-size-small)', 'var(--font-line-height-small)'],
                body: ['var(--font-size-body)', 'var(--font-line-height-body)'],
                title: ['var(--font-size-title)', 'var(--font-line-height-title)'],
                display: ['var(--font-size-display)', 'var(--font-line-height-display)'],
            },
            // No borderRadius here: Tailwind's rounded-md is 6 px, the token's
            // radius-md is 4 px, and step 1 changes nothing on screen. The
            // components use the token through their own classes.
            minWidth: {
                '72': '18rem',
                '96': '24rem',
                '104': '26rem',
                '120': '30rem',
            },
            width: {
                '104': '26rem',
                '120': '30rem',
            },
        },
    },
};
