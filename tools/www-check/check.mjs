#!/usr/bin/env node
// www-check: a safety net for removing code from www/ without breaking the page.
//
// Run from the repository root, with Node 22 or later and nothing installed:
//
//     node tools/www-check/check.mjs
//
// It fails (exit 1) when:
//   - a `$store.atc.X` read by index.html, or a store access in another www/
//     script (`store.X`, `this.store.X`, `Alpine.store('atc').X`, ...), names
//     something the Alpine store defined in app.js does not have;
//   - a store method reads `this.X` and X is not on the store;
//   - an Alpine expression in index.html calls a bare function that no loaded
//     script, x-data component or browser global defines;
//   - a local <script src> of index.html points to a missing file;
//   - a www/ stylesheet has an unclosed comment or unbalanced braces;
//   - `node --check` fails on a www/ script, or `node --test www/par/` fails.
//
// The store is not parsed: app.js is evaluated in a sandbox with stubbed
// browser globals, and the object handed to Alpine.store('atc', ...) is
// captured before anything else in the alpine:init handler runs. Getters are
// listed, never invoked. `--quick` skips node --check and the PAR tests.

import { readFileSync, existsSync, readdirSync, statSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { join, dirname, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import vm from 'node:vm';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const WWW = join(ROOT, 'www');
const quick = process.argv.includes('--quick');
const problems = [];

function walk(dir) {
    return readdirSync(dir).flatMap((name) => {
        const p = join(dir, name);
        return statSync(p).isDirectory() ? walk(p) : [p];
    });
}

const jsFiles = walk(WWW).filter((f) => f.endsWith('.js'));
const html = readFileSync(join(WWW, 'index.html'), 'utf8');
const appSource = readFileSync(join(WWW, 'app.js'), 'utf8');

// ---------------------------------------------------------------- the store

class StoreCaptured extends Error {}

function captureStore() {
    const listeners = {};
    const noop = () => {};
    const element = new Proxy(noop, { get: () => element, apply: () => element });
    const sandbox = {
        console: { log: noop, info: noop, warn: noop, error: noop, debug: noop },
        location: { protocol: 'http:', hostname: 'localhost', port: '8000', href: 'http://localhost:8000/' },
        navigator: {},
        localStorage: { getItem: () => null, setItem: noop, removeItem: noop },
        document: {
            addEventListener: (type, cb) => { (listeners[type] ||= []).push(cb); },
            getElementById: () => null,
            querySelector: () => null,
            querySelectorAll: () => [],
            createElement: () => element,
        },
        addEventListener: noop,
        setTimeout: () => 0, clearTimeout: noop, setInterval: () => 0, clearInterval: noop,
        WebSocketClient: class { constructor() {} },
        isSecureContext: false,
    };
    sandbox.window = sandbox;
    let store = null;
    sandbox.Alpine = {
        store(name, value) {
            if (name === 'atc' && value) { store = value; throw new StoreCaptured(); }
            return store;
        },
        effect: noop,
    };
    const context = vm.createContext(sandbox);
    // In the page's order: what app.js reads at load, the store's domains
    // (each pushes to window.AtcStore), the x-data factories of their own, app.js.
    const before = ['tile-cache.js', ...readdirSync(join(WWW, 'store')).filter((f) => f.endsWith('.js')).sort().map((f) => 'store/' + f),
        'accounts-panel.js', 'usage-panel.js', 'server-settings.js', 'altitude/altitude-filter.js', 'altitude/altitude-legend.js'];
    for (const f of before) {
        vm.runInContext(readFileSync(join(WWW, f), 'utf8'), context, { filename: `www/${f}` });
    }
    vm.runInContext(appSource, context, { filename: 'www/app.js' });
    for (const cb of listeners['alpine:init'] || []) {
        try { cb(); } catch (e) { if (!(e instanceof StoreCaptured)) throw e; }
        if (store) break;
    }
    if (!store) throw new Error("app.js never called Alpine.store('atc', {...})");
    return { store, context };
}

const { store, context } = captureStore();
const descriptors = Object.getOwnPropertyDescriptors(store);
const storeKeys = new Set(Object.keys(descriptors));

// Fields the code adds to the store after it is created, by assignment
// (`this.X = ...` in a store method, `store.X = ...` anywhere).
const STORE_REF = String.raw`(?:\bthis\.store|\bmanager\.store|\bmapManager\.store|Alpine\.store\(\s*['"]atc['"]\s*\)|(?<![.\w$])store)\s*\??\.\s*([A-Za-z_$][\w$]*)`;
const assigned = new Set();
const methodSources = [];
for (const [key, d] of Object.entries(descriptors)) {
    for (const fn of [d.value, d.get, d.set]) {
        if (typeof fn === 'function') methodSources.push([key, stripStrings(fn.toString(), { comments: true })]);
    }
}
for (const [, src] of methodSources) {
    for (const m of src.matchAll(/\bthis\.([A-Za-z_$][\w$]*)\s*(?:=(?!=)|\+\+|--|[-+*/|&]=)/g)) assigned.add(m[1]);
}
for (const f of jsFiles) {
    const src = stripStrings(readFileSync(f, 'utf8'), { comments: true, strings: false });
    for (const m of src.matchAll(new RegExp(STORE_REF + String.raw`\s*=(?!=)`, 'g'))) assigned.add(m[1]);
}
const known = (name) => storeKeys.has(name) || assigned.has(name);

// ---------------------------------------------------------------- index.html

const lineOf = (text, index) => text.slice(0, index).split('\n').length;

// Alpine attribute values: x-*, :attr, @event.
const attrs = [];
for (const m of html.matchAll(/\s((?:x-[\w:.-]+|:[\w.-]+|@[\w.:-]+))\s*=\s*"([^"]*)"/g)) {
    attrs.push({ name: m[1], value: m[2], line: lineOf(html, m.index) });
}

const htmlStoreNames = new Set();
for (const m of html.matchAll(/\$store\.atc\s*\??\.\s*([A-Za-z_$][\w$]*)/g)) {
    htmlStoreNames.add(m[1]);
    if (!known(m[1])) problems.push(`index.html:${lineOf(html, m.index)}: $store.atc.${m[1]} is not on the store`);
}

// Blank out comments and string literal text, keeping ${...} inside template
// literals. Regex literals are not recognised; none in www/ upsets this.
function stripStrings(src, { comments = false, strings = true } = {}) {
    let out = '';
    const stack = []; // open template literals, each with its ${ depth
    for (let i = 0; i < src.length; i++) {
        const c = src[i];
        const top = stack[stack.length - 1];
        if (top && top.depth === 0) { // inside template text
            if (c === '\\') { i++; continue; }
            if (c === '`') { stack.pop(); out += ' '; continue; }
            if (c === '$' && src[i + 1] === '{') { top.depth = 1; out += ' '; i++; continue; }
            if (c === '\n') out += c;
            continue;
        }
        if (top) {
            if (c === '{') top.depth++;
            if (c === '}') { top.depth--; if (top.depth === 0) { out += ' '; continue; } }
        }
        if (comments && c === '/' && src[i + 1] === '/') {
            while (i < src.length && src[i] !== '\n') i++;
            out += '\n'; continue;
        }
        if (comments && c === '/' && src[i + 1] === '*') {
            const end = src.indexOf('*/', i + 2);
            const skipped = src.slice(i, end < 0 ? src.length : end + 2);
            out += skipped.replace(/[^\n]/g, '');
            i = end < 0 ? src.length : end + 1; continue;
        }
        if (c === "'" || c === '"') {
            let j = i + 1;
            while (j < src.length && src[j] !== c && src[j] !== '\n') j += src[j] === '\\' ? 2 : 1;
            out += strings ? ' ' + src.slice(i, j).replace(/[^\n]/g, '') : src.slice(i, j + 1);
            i = j; continue;
        }
        if (c === '`') { stack.push({ depth: 0 }); out += ' '; continue; }
        out += c;
    }
    return out;
}

// What a bare call in an Alpine expression may resolve to.
const callable = new Set([
    // keywords that precede a parenthesis
    'if', 'for', 'while', 'switch', 'catch', 'function', 'return', 'typeof', 'await', 'new', 'in', 'of',
    // browser globals Node lacks
    'fetch', 'alert', 'confirm', 'prompt', 'requestAnimationFrame', 'cancelAnimationFrame',
]);
for (const name of Object.getOwnPropertyNames(globalThis)) callable.add(name);
// Globals defined by the page's own scripts.
for (const f of jsFiles) {
    const src = stripStrings(readFileSync(f, 'utf8'), { comments: true, strings: false });
    for (const m of src.matchAll(/^(?:async\s+)?function\s+([A-Za-z_$][\w$]*)|^(?:const|let|var|class)\s+([A-Za-z_$][\w$]*)|\bwindow\.([A-Za-z_$][\w$]*)\s*=(?!=)/gm)) {
        callable.add(m[1] || m[2] || m[3]);
    }
}
// x-data components: a global factory (its returned object's keys) or an inline object.
for (const a of attrs.filter((a) => a.name === 'x-data' && a.value.trim())) {
    const factory = a.value.trim().match(/^([A-Za-z_$][\w$]*)\s*\(\s*\)$/);
    if (factory) {
        const fn = context[factory[1]];
        if (typeof fn !== 'function') { problems.push(`index.html:${a.line}: x-data calls ${factory[1]}(), which no script defines`); continue; }
        Object.keys(Object.getOwnPropertyDescriptors(fn())).forEach((k) => callable.add(k));
    } else {
        for (const m of stripStrings(a.value).matchAll(/([A-Za-z_$][\w$]*)\s*(?:\(|:)/g)) callable.add(m[1]);
    }
}

const ALPINE_MAGIC = new Set(['$nextTick', '$watch', '$dispatch', '$el', '$refs', '$store', '$event', '$data', '$id', '$root']);
let bareCalls = 0;
for (const a of attrs) {
    if (a.name === 'x-data') continue;
    const expr = stripStrings(a.value);
    for (const m of expr.matchAll(/(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(/g)) {
        const name = m[1];
        bareCalls++;
        if (name.startsWith('$') ? ALPINE_MAGIC.has(name) : callable.has(name)) continue;
        problems.push(`index.html:${a.line}: ${a.name}="..." calls ${name}(), which nothing defines`);
    }
}

let scripts = 0;
for (const m of html.matchAll(/<script\b[^>]*\bsrc\s*=\s*"([^"]+)"/g)) {
    const src = m[1];
    if (/^(?:https?:)?\/\//.test(src)) continue;
    scripts++;
    if (!existsSync(join(WWW, src.replace(/^\//, '').split(/[?#]/)[0]))) {
        problems.push(`index.html:${lineOf(html, m.index)}: <script src="${src}"> points to a missing file`);
    }
}

// ---------------------------------------------------------------- the other scripts

const otherNames = new Set();
for (const f of jsFiles) {
    if (f.endsWith('.test.js')) continue;
    const src = stripStrings(readFileSync(f, 'utf8'), { comments: true, strings: false });
    const rel = relative(ROOT, f);
    for (const m of src.matchAll(new RegExp(STORE_REF, 'g'))) {
        if (f.endsWith(join('www', 'app.js')) && !/Alpine\.store/.test(m[0]) && !/^store/.test(m[0])) continue;
        otherNames.add(m[1]);
        if (!known(m[1])) problems.push(`${rel}:${lineOf(src, m.index)}: store.${m[1]} is not on the store`);
    }
}

// `this.X` read inside a store method must be on the store. The exceptions
// are defects already there on main, left to the rewrite of their area; one
// that disappears must leave this list too, so that the list stays true.
const KNOWN_BROKEN = new Map([
]);
let thisReads = 0;
const brokenSeen = new Set();
for (const [key, src] of methodSources) {
    for (const m of src.matchAll(/\bthis\.([A-Za-z_$][\w$]*)/g)) {
        thisReads++;
        if (known(m[1])) continue;
        const id = `${key} reads this.${m[1]}`;
        if (KNOWN_BROKEN.has(id)) { brokenSeen.add(id); continue; }
        problems.push(`www/app.js: store.${id}, which is not on the store`);
    }
}
for (const id of KNOWN_BROKEN.keys()) {
    if (!brokenSeen.has(id)) problems.push(`tools/www-check/check.mjs: KNOWN_BROKEN "${id}" no longer happens; remove it from the list`);
}

// ---------------------------------------------------------------- style.css

// Not a CSS parser: comments must close and braces balance, which is what a
// removal by line range gets wrong.
for (const f of walk(WWW).filter((f) => f.endsWith('.css'))) {
    const css = readFileSync(f, 'utf8');
    const rel = relative(ROOT, f);
    let depth = 0;
    for (let i = 0; i < css.length; i++) {
        if (css[i] === '*' && css[i + 1] === '/') { problems.push(`${rel}:${lineOf(css, i)}: */ without an opening /*`); break; }
        if (css[i] === '/' && css[i + 1] === '*') {
            const end = css.indexOf('*/', i + 2);
            if (end < 0) { problems.push(`${rel}:${lineOf(css, i)}: /* never closed`); break; }
            i = end + 1; continue;
        }
        if (css[i] === '{') depth++;
        if (css[i] === '}' && --depth < 0) { problems.push(`${rel}:${lineOf(css, i)}: } without a {`); break; }
    }
    if (depth > 0) problems.push(`${rel}: ${depth} { never closed`);
}

// ---------------------------------------------------------------- compiled tailwind

// www/vendor/tailwind.css holds the classes the page used when it was built
// (tools/tailwind/build.sh): one added since renders unstyled. Its first line
// carries a fingerprint of the sources it was built from, recomputed here.
{
    const css = join(WWW, 'vendor', 'tailwind.css');
    if (!existsSync(css)) {
        problems.push('www/vendor/tailwind.css is missing: run tools/tailwind/build.sh');
    } else {
        const built = (readFileSync(css, 'utf8').match(/^\/\* sources ([0-9a-f]{16}) \*\//) || [])[1];
        const sources = [join(WWW, 'index.html'), ...jsFiles.filter((f) => !f.includes('/vendor/'))].sort();
        const h = createHash('sha256');
        for (const f of sources) { h.update(relative(WWW, f) + '\0'); h.update(readFileSync(f)); }
        if (h.digest('hex').slice(0, 16) !== built) problems.push('www/vendor/tailwind.css was built from other sources: run tools/tailwind/build.sh');
    }
}

// www/tokens.css and www/components.css are copies of the design system
// (docs-fr/design/systeme/project/), written by tools/design/ds-export.mjs
// --www and never edited by hand (04-migration.md, rule 1): a copy that is
// missing, retouched or behind its source fails here. Since step 1 of the
// migration, index.html links both: a missing copy is a problem too.
{
    const copies = ['tokens.css', 'components.css'].map((f) => join(WWW, f));
    const linked = copies.filter((f) => html.includes(`href="${relative(WWW, f)}"`));
    for (const f of linked) if (!existsSync(f)) problems.push(`${relative(ROOT, f)} is linked by index.html but missing: run node tools/design/ds-export.mjs --www`);
    if (linked.length || copies.some((f) => existsSync(f))) {
        const r = spawnSync(process.execPath, [join(ROOT, 'tools', 'design', 'ds-export.mjs'), '--check', 'www'], { cwd: ROOT, encoding: 'utf8' });
        if (r.status !== 0) problems.push(`the design system's copies in www/ differ from their source:\n${(r.stdout + r.stderr).trim()}\nrun node tools/design/ds-export.mjs --www`);
    }
}

// ---------------------------------------------------------------- syntax and tests

if (!quick) {
    for (const f of jsFiles) {
        const r = spawnSync(process.execPath, ['--check', f], { encoding: 'utf8' });
        if (r.status !== 0) problems.push(`node --check ${relative(ROOT, f)}:\n${r.stderr.trim()}`);
    }
    const tests = ['par', 'altitude', 'timeline', 'phase', 'now', 'story', 'map'].flatMap((d) => walk(join(WWW, d)))
        .concat(readdirSync(WWW).map((f) => join(WWW, f)))
        .filter((f) => f.endsWith('.test.js'));
    const r = spawnSync(process.execPath, ['--test', ...tests], { encoding: 'utf8', cwd: ROOT });
    if (r.status !== 0) problems.push(`node --test www/par/ www/altitude/ www/timeline/ www/phase/ www/now/ www/story/ www/map/ www/*.test.js failed:\n${(r.stdout + r.stderr).trim().split('\n').slice(-30).join('\n')}`);
}

console.log(`store: ${storeKeys.size} properties defined, ${[...assigned].filter((n) => !storeKeys.has(n)).length} added by assignment`);
console.log(`index.html: ${htmlStoreNames.size} store names, ${bareCalls} bare calls, ${scripts} local scripts`);
console.log(`other scripts: ${otherNames.size} store names; store methods: ${thisReads} this.X reads, ${brokenSeen.size} known broken`);
if (!quick) console.log(`node --check on ${jsFiles.length} files, node --test www/par/ www/altitude/ www/timeline/ www/phase/ www/now/ www/story/ www/map/ www/*.test.js`);
if (problems.length) {
    console.error(`\n${problems.length} problem(s):\n` + problems.map((p) => '  ' + p).join('\n'));
    process.exit(1);
}
console.log('OK');
