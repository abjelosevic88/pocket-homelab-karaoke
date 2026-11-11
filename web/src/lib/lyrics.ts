// The words of a song: LRC with a time per line ("[01:02.30]a line") and,
// in "enhanced" LRC, per word ("<01:02.80>word"). Plain lines have no
// times. internal/server/lyrics.go reads the same format.

export interface Word { t: number; text: string }
export interface Line { t: number; end: number; text: string; words: Word[] }

const reTime = /\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]/g
const reWord = /<(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?>/g
const reTag = /^\[(ti|ar|al|by|offset|length|re|ve|au|#):([^\]]*)\]\s*$/

function ms(m: string, s: string, frac?: string): number {
  let f = frac || ''
  while (f.length < 3) f += '0'
  return Number(m) * 60000 + Number(s) * 1000 + Number(f.slice(0, 3))
}

export function stamp(t: number): string {
  t = Math.max(0, Math.round(t))
  const m = Math.floor(t / 60000), s = Math.floor(t / 1000) % 60, c = Math.floor((t % 1000) / 10)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}.${String(c).padStart(2, '0')}`
}

/** Reads LRC (or plain text) into lines; `synced` says whether there were times. */
export function parseLyrics(text: string): { lines: Line[]; synced: boolean } {
  const out: Line[] = []
  let synced = false
  let offset = 0
  for (const raw of (text || '').replace(/\r/g, '').split('\n')) {
    const line = raw.trim()
    const tag = line.match(reTag)
    if (tag) { if (tag[1] === 'offset') offset = Number(tag[2]) || 0; continue }
    const times = [...line.matchAll(reTime)]
    const body = line.replace(reTime, '')
    if (!times.length) {
      if (!body.trim() && !out.length) continue
      out.push({ t: -1, end: -1, text: body.replace(reWord, '').replace(/\s+/g, ' ').trim(), words: [] })
      continue
    }
    synced = true
    // words with their own times
    const words: Word[] = []
    const parts = body.split(reWord)
    // split with capture groups: [before, m, s, f, text, m, s, f, text…]
    if (parts.length > 1) {
      for (let i = 1; i + 3 < parts.length + 1; i += 4) {
        const t = ms(parts[i], parts[i + 1], parts[i + 2]) - offset
        const w = parts[i + 3] ?? ''
        if (w.trim() || words.length) words.push({ t, text: w })
      }
    }
    const text = body.replace(reWord, '').replace(/\s+/g, ' ').trim()
    for (const tm of times) {
      const t = Math.max(0, ms(tm[1], tm[2], tm[3]) - offset)
      const shift = words.length ? t - words[0].t : 0
      out.push({ t, end: -1, text, words: times.length > 1 ? words.map(w => ({ t: w.t + shift, text: w.text })) : words })
    }
  }
  let lines = out
  if (synced) {
    lines = out.filter(l => l.t >= 0).sort((a, b) => a.t - b.t)
    for (let i = 0; i < lines.length; i++) {
      const next = lines[i + 1]
      const guess = lines[i].t + Math.max(2500, Math.min(8000, lines[i].text.length * 260))
      lines[i].end = next ? Math.min(next.t, guess + 1500) : guess
      if (!lines[i].text) lines[i].end = next ? next.t : lines[i].t
    }
  }
  while (lines.length && !lines[lines.length - 1].text) lines.pop()
  return { lines, synced }
}

/** Where a time falls: the line being sung (or -1 before the first), and how far through it (0..1). */
export function lineAt(lines: Line[], t: number): number {
  let lo = 0, hi = lines.length - 1, at = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (lines[mid].t <= t) { at = mid; lo = mid + 1 } else hi = mid - 1
  }
  return at
}

/** How much of a line has been sung, 0..1, by its words' times if it has them. */
export function lineProgress(l: Line, t: number): number {
  if (t <= l.t) return 0
  if (l.words.length > 1) {
    const total = l.text.length || 1
    let done = 0
    for (let i = 0; i < l.words.length; i++) {
      const w = l.words[i]
      const wEnd = l.words[i + 1]?.t ?? Math.max(w.t + 400, l.end)
      const len = w.text.length
      if (t >= wEnd) { done += len; continue }
      if (t > w.t) done += len * Math.min(1, (t - w.t) / Math.max(120, wEnd - w.t))
      break
    }
    return Math.min(1, done / total)
  }
  const end = l.end > l.t ? l.end : l.t + 3000
  return Math.min(1, (t - l.t) / (end - l.t))
}

/** Writes lines with times back as LRC (the tap-in-time editor). */
export function toLRC(lines: { t: number; text: string }[], head?: { title?: string; artist?: string }): string {
  const out: string[] = []
  if (head?.title) out.push(`[ti:${head.title}]`)
  if (head?.artist) out.push(`[ar:${head.artist}]`)
  for (const l of lines) out.push(l.t >= 0 ? `[${stamp(l.t)}]${l.text}` : l.text)
  return out.join('\n')
}

/** The words without any times. */
export function plainText(text: string): string {
  return parseLyrics(text).lines.map(l => l.text).join('\n')
}
