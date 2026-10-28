// The guest's page across a restart of the server (fiche S, 26/10): signed in
// through the real form, every stream playing, the server stopped then
// started; what the page does, timed against the moment the server answers
// /api/v1/health again.
//
//   node tools/restart-bench/restart.mjs <ctl> <seconds down> [<label>]
//
// <ctl> is a script taking "stop" (SIGINT, returns once the process is gone)
// and "start" (returns once /api/v1/health answers). The instance: the
// example configuration as tools/startup-bench/run.sh adapts it, sign-in on
// with a viewer account, server.audio_replay = "all", every source on
// mp3loop.py. PROXY=1 sends X-Forwarded-For, for a server whose
// trusted_proxies holds 127.0.0.1/32: it then serves the streams on the
// page's origin, as through the owner's Traefik. Without it, the streams on
// the additional ports get no cookie from the page, before any restart.
// BASE, GUEST and PASS name the server and the account (defaults below).
// Prints one JSON object: the delays, the 401s, the timeline.
import { createRequire } from 'module';
import { execFileSync } from 'child_process';
const { chromium } = createRequire(import.meta.url)('playwright');
const [srv, downS = '3', label = 'run'] = process.argv.slice(2);
const BASE = process.env.BASE || 'http://127.0.0.1:18800';
const sleep = ms => new Promise(r => setTimeout(r, ms));

const browser = await chromium.launch({ args: ['--autoplay-policy=no-user-gesture-required'] });
const proxied = process.env.PROXY === '1'; // as through Traefik: the server serves the streams on the page's origin
const page = await (await browser.newContext(proxied ? { extraHTTPHeaders: { 'X-Forwarded-For': '192.0.2.10' } } : {})).newPage();
const unauthorized = [];
page.on('response', r => { if (r.status() === 401) unauthorized.push([Date.now(), r.url().replace(BASE, '')]); });
await page.addInitScript(() => {
  window.__log = [];
  const log = (what) => window.__log.push([Date.now(), what]);
  const WS = window.WebSocket;
  window.WebSocket = function (...a) {
    const ws = new WS(...a);
    ws.addEventListener('open', () => log('ws:open'));
    ws.addEventListener('close', () => log('ws:close'));
    return ws;
  };
  window.WebSocket.prototype = WS.prototype;
  Object.assign(window.WebSocket, { CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3 });
  const patch = () => {
    if (!window.LiveStreamPlayer || window.LiveStreamPlayer.prototype.__patched) return;
    const emit = window.LiveStreamPlayer.prototype._emit;
    window.LiveStreamPlayer.prototype._emit = function (t) { log('player:' + t + ' ' + String(this.src).replace(/^.*\/stream\/([^?]*).*$/, '$1') + (t === 'error' ? ' ' + (this.error && this.error.message) : '')); return emit.call(this, t); };
    window.LiveStreamPlayer.prototype.__patched = true;
  };
  document.addEventListener('DOMContentLoaded', patch);
  setInterval(patch, 50);
});

await page.goto(BASE + '/');
await page.fill('#coatc-user', process.env.GUEST || 'guest');
await page.fill('#coatc-pass', process.env.PASS || 'motdepasse-essai-1');
await Promise.all([page.waitForNavigation(), page.click('button[type=submit]')]);
await page.waitForFunction(() => window.Alpine && Alpine.store('atc') && Alpine.store('atc').initialDataLoaded, null, { timeout: 30000 });
await page.mouse.click(400, 300); // the click a browser asks before sound
await page.evaluate(() => { const s = Alpine.store('atc'); if (!s.radiosStarted) s.startAllRadios(); });
await page.waitForFunction(() => window.__log.some(e => e[1].startsWith('player:playing')), null, { timeout: 20000 });
await sleep(4000);
const players = [...new Set((await page.evaluate(() => window.__log)).filter(e => e[1].startsWith('player:playing')).map(e => e[1].split(' ')[1]))];
await sleep(3000);

const banner = '[role=alert].cl-banner';
const bannerSeen = [];
let shown = false;
const watch = setInterval(async () => {
  try {
    const v = await page.isVisible(banner);
    if (v !== shown) { shown = v; bannerSeen.push([Date.now(), v ? 'banner:shown' : 'banner:hidden']); }
  } catch { /* navigating */ }
}, 100);

const tStop = Date.now();
execFileSync(srv, ['stop']);
const tDown = Date.now();
await sleep(Number(downS) * 1000);
const tStart = Date.now();
execFileSync(srv, ['start']); // returns once /api/v1/health answers
const tUp = Date.now();

await page.waitForFunction(([t, n]) => {
  const after = window.__log.filter(e => e[0] >= t);
  return after.some(e => e[1] === 'ws:open') && new Set(after.filter(e => e[1].startsWith('player:playing')).map(e => e[1].split(' ')[1])).size >= n;
}, [tUp, players.length], { timeout: 60000, polling: 100 }).catch(() => {});
await sleep(2000);
clearInterval(watch);

const log = (await page.evaluate(() => window.__log)).concat(bannerSeen).sort((a, b) => a[0] - b[0]);
const firstAfter = (what, t = tUp) => { const e = log.find(e => e[0] >= t && e[1] === what); return e ? e[0] - tUp : null; };
const signInShown = await page.isVisible('#coatc-user').catch(() => false);
console.log(JSON.stringify({
  label, down_s: Number(downS), players_playing_before: players,
  stop_took_ms: tDown - tStop, start_took_ms: tUp - tStart,
  ws_open_after_up_ms: firstAfter('ws:open'),
  sound_after_up_ms: Object.fromEntries(players.map(f => [f, firstAfter('player:playing ' + f)])), // null: not back
  proxied,
  banner_shown_after_stop_ms: firstAfter('banner:shown', tStop) === null ? null : firstAfter('banner:shown', tStop) + tUp - tStop,
  banner_hidden_after_up_ms: firstAfter('banner:hidden'),
  sign_in_form_shown: signInShown, url: page.url().replace(BASE, ''),
  unauthorized_after_stop: unauthorized.filter(u => u[0] >= tStop).length,
  unauthorized_sample: unauthorized.filter(u => u[0] >= tStop).slice(0, 3).map(u => u[1]),
  timeline: log.filter(e => e[0] >= tStop - 100).map(e => [e[0] - tStop, e[1]]),
}, null, 1));
await browser.close();
