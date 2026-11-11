// README screenshots and a browser smoke test against a demo instance:
//
//   make build && DATA_DIR=/tmp/kar-demo DEMO_DATA=true AUTH_MODE=none LOOKUPS=off HOST=127.0.0.1 PORT=18273 ./pocket-karaoke &
//   CDP=http://127.0.0.1:9343 BASE=http://127.0.0.1:18273 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
// Start Chrome with --autoplay-policy=no-user-gesture-required for the singing shots.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18273'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false, w = 1440, h = 900 } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: w, height: h },
    deviceScaleFactor: 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|youtube|ytimg|ERR_/.test(m.text())) errors.push(m.text()) })
  return { ctx, page }
}
async function go(page, path, wait = 1400) {
  await page.goto(base + path, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(wait)
}
async function shot(page, name) {
  if (!shots || (only && !only.split(',').includes(name))) return
  for (let i = 0; ; i++) {
    try { await page.bringToFront(); await page.screenshot({ path: dir + name + '.png', animations: 'disabled' }); return } catch (e) { if (i >= 3) throw e; await page.waitForTimeout(700) }
  }
}
const api = (page, path, method = 'GET', body) => page.evaluate(([p, m, b]) => fetch('/api/v1' + p, { method: m, headers: { 'Content-Type': 'application/json' }, body: b ? JSON.stringify(b) : undefined }).then(r => r.status === 204 ? {} : r.json()), [path, method, body])

const { page } = await session()
await go(page, '/', 2000)
assert.ok((await page.content()).includes('Friday karaoke'), 'the live night shows on the home page')
await shot(page, 'desktop-home')

await go(page, '/songs')
assert.ok((await page.content()).includes('Bohemian Rhapsody'), 'the songs list')
await shot(page, 'desktop-songs')

const songs = (await api(page, '/songs?limit=100')).songs
const twinkle = songs.find(s => s.title.startsWith('Twinkle'))
const grace = songs.find(s => s.title === 'Amazing Grace')
await go(page, `/songs/${grace.id}`)
assert.ok((await page.content()).includes('Amazing grace, how sweet the sound'), 'the song page shows the words')
await shot(page, 'desktop-song')

const nights = (await api(page, '/nights')).nights
const live = nights.find(n => n.status === 'live')
await go(page, `/nights/${live.id}`, 1800)
assert.ok((await page.content()).includes('Up next on the screen'), 'the host page shows the screen state')
await shot(page, 'desktop-night')

await go(page, '/history', 1600)
await shot(page, 'desktop-history')

// the big screen: Jelena is up next with Twinkle (a song that really plays)
const scr = await session({ w: 1600, h: 900 })
await go(scr.page, `/screen/${live.screen_key}`, 1500)
assert.ok((await scr.page.content()).includes('SING42'), 'the big screen shows the join code')
await shot(scr.page, 'desktop-screen-next')
await scr.page.mouse.click(800, 450)
await scr.page.waitForTimeout(300)
await api(page, `/nights/${live.id}/control`, 'POST', { action: 'play' })
await scr.page.waitForTimeout(5600)
assert.ok(await scr.page.locator('.lyr-line.now .lyr-w').count() > 0, 'the words show on the big screen while it plays')
await shot(scr.page, 'desktop-screen')
await api(page, `/nights/${live.id}/control`, 'POST', { action: 'pause' })

// sing alone on this device
const sing = await session({ w: 1440, h: 900 })
await go(sing.page, `/sing/${twinkle.id}`, 1200)
await sing.page.locator('.sing-bar .btn.round').click()
await sing.page.waitForTimeout(8900)
assert.ok(await sing.page.locator('.lyr-line.now .lyr-w').count() > 0, 'the words show on the Sing page')
await sing.page.locator('button[aria-label="Sound and microphone"]').click()
await sing.page.waitForTimeout(400)
await shot(sing.page, 'desktop-sing')
await sing.page.locator('.sing-bar .btn.round').click()

// a guest's phone
const ph = await session({ phone: true })
await go(ph.page, '/j/SING42', 1200)
await ph.page.fill('input', 'Zoran')
await ph.page.locator('button[type=submit]').click()
await ph.page.waitForTimeout(1500)
assert.ok((await ph.page.content()).includes('Pick a song'), 'a guest joins with a name')
await shot(ph.page, 'phone-join')
await ph.page.locator('.g-row').first().click()
await ph.page.waitForTimeout(500)
await ph.page.locator('.modal .btn.primary').click()
await ph.page.waitForTimeout(1200)
assert.ok((await ph.page.content()).includes('Zoran'), 'the guest is in the queue')
await shot(ph.page, 'phone-queue')

// the owner on a phone
const me = await session({ phone: true })
await go(me.page, `/nights/${live.id}`, 1600)
await shot(me.page, 'phone-night')
await go(me.page, '/', 1500)
await shot(me.page, 'phone-home')

// dark
const dk = await session({ dark: true })
await go(dk.page, '/songs', 1500)
await shot(dk.page, 'desktop-songs-dark')

await browser.close().catch(() => {})
if (errors.length) { console.error('browser errors:\n' + errors.join('\n')); process.exit(1) }
console.log('ok')
