// One song: its details, the words (typed, looked up, timed by tapping,
// moved earlier or later), a copy of a YouTube song, its song books and who
// sang it.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, upload, V1 } from '../lib/api'
import type { Book, LyricsCandidate, Night, Performance, Song } from '../lib/types'
import { KEY_LABEL, LANGS, SOURCE, fmtLen, screenSettingsFrom, stageSong, wordsOf } from '../lib/karaoke'
import { parseLyrics, plainText, stamp, toLRC } from '../lib/lyrics'
import { useApp } from '../lib/app-context'
import { announceChange, useAppState } from '../lib/data'
import { useCrumb } from '../components/Layout'
import { Confirm, Empty, Field, Loading, Modal, Segmented, Spinner, Switch, Tabs, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { SongCover } from './Songs'
import { Stage, type StageHandle } from '../components/Stage'
import { ago, fmtDateTime } from '../lib/format'

export default function SongPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [data, setData] = useState<{ song: Song; performances: Performance[]; copy_possible: boolean } | null>(null)
  const [missing, setMissing] = useState(false)
  const [books, setBooks] = useState<Book[]>([])
  const [live, setLive] = useState<Night | null>(null)
  const [del, setDel] = useState(false)
  const load = useCallback(() => {
    api.get<{ song: Song; performances: Performance[]; copy_possible: boolean }>(`${V1}/songs/${id}`).then(setData).catch(() => setMissing(true))
  }, [id])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    api.get<{ books: Book[] }>(`${V1}/books`).then(r => setBooks(r.books)).catch(() => {})
    api.get<{ nights: Night[] }>(`${V1}/nights`).then(r => setLive(r.nights.find(n => n.status === 'live') || null)).catch(() => {})
  }, [])
  // a copy being made: follow it
  useEffect(() => {
    if (data?.song.copy_state !== 'waiting' && data?.song.copy_state !== 'working') return
    const t = setInterval(load, 3000)
    return () => clearInterval(t)
  }, [data?.song.copy_state, load])
  useCrumb(data?.song.title, '/songs')
  if (missing) return <Empty title="This song is not here" icon="music"><Link to="/songs">All songs</Link></Empty>
  if (!data) return <Loading />
  const s = data.song
  const save = async (patch: Partial<Song>) => {
    try { const n = await api.put<Song>(`${V1}/songs/${s.id}`, patch); setData({ ...data, song: n }); announceChange(); return true } catch (e) { toast(errMsg(e), 'err'); return false }
  }
  const copy = async (keep: boolean) => {
    try {
      const n = keep ? await api.post<Song>(`${V1}/songs/${s.id}/copy`) : await api.del<Song>(`${V1}/songs/${s.id}/copy`)
      setData({ ...data, song: n })
      toast(keep ? 'Keeping a copy: it plays from this server when it is done' : 'The copy is gone; it plays from YouTube again')
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const queue = async () => {
    if (!live) return
    try { await api.post(`${V1}/nights/${live.id}/queue`, { song_id: s.id }); toast(`In the queue of “${live.title}”`, 'ok', { label: 'Open', run: () => navigate(`/nights/${live.id}`) }) } catch (e) { toast(errMsg(e), 'err') }
  }
  const words = wordsOf(s)
  return (
    <div className="page narrow">
      <div className="song-head">
        <SongCover s={s} size={96} />
        <div className="grow min0">
          <h1 className="song-h1">{s.title}</h1>
          <div className="muted">{s.artist || 'Unknown artist'}{s.duration_ms ? ` · ${fmtLen(s.duration_ms)}` : ''}</div>
          <div className="row wrap small mt-s">
            <span className="src-tag"><Icon name={SOURCE[s.source].icon} size={13} />{SOURCE[s.source].label}</span>
            <span className={`words-tag ${words.ok ? '' : 'missing'}`}>{words.text}</span>
            <span className="badge">{s.kind === 'karaoke' ? 'Karaoke track' : 'With the voice'}</span>
            {s.added_by && <span className="badge">asked for by {s.added_by}</span>}
            {s.plays > 0 && <span className="muted">sung {s.plays}×{s.last_sung_at ? `, last ${ago(s.last_sung_at)}` : ''}</span>}
          </div>
        </div>
      </div>
      <div className="row wrap">
        <Link className="btn primary lg" to={`/sing/${s.id}`}><Icon name="mic" size={16} />Sing it</Link>
        {live && <button type="button" className="btn lg" onClick={queue}><Icon name="list-plus" size={16} />Put it in tonight’s queue</button>}
        <button type="button" className={`btn ${s.favourite ? 'fav-on' : ''}`} onClick={() => save({ favourite: !s.favourite })}><Icon name="star" size={15} />{s.favourite ? 'A favourite' : 'Favourite'}</button>
        <span className="spacer" />
        <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={15} />Remove</button>
      </div>

      {s.source === 'youtube' && <CopyCard s={s} possible={data.copy_possible} onCopy={copy} />}

      <Lyrics s={s} onSave={save} />
      <Details s={s} onSave={save} reload={load} />
      <BooksCard s={s} books={books} onChange={load} />

      <section className="card pad-card">
        <h3 className="card-h">Who sang it</h3>
        {data.performances.length === 0 ? <p className="muted small">Nobody yet.</p> : <div className="perf-list">{data.performances.map(p => <PerfRow key={p.id} p={p} />)}</div>}
      </section>
      {del && <Confirm title="Remove this song?" message={<>“{s.title}” goes from your songs{s.has_media ? ', with its file' : ''}. Who sang it stays in the history.</>} confirmLabel="Remove"
        onCancel={() => setDel(false)} onConfirm={async () => { try { await api.del(`${V1}/songs/${s.id}`); announceChange(); navigate('/songs') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}

export function PerfRow({ p, showSong = false }: { p: Performance; showSong?: boolean }) {
  return (
    <div className="perf-row">
      <span className={`score-dot ${p.score === null ? 'none' : ''}`}>{p.score ?? '—'}</span>
      <div className="grow min0">
        <div className="ellipsis">{showSong ? <>{p.song_id ? <Link to={`/songs/${p.song_id}`}>{p.title}</Link> : p.title} <span className="muted">· {p.singer}</span></> : <b>{p.singer || 'Someone'}</b>}</div>
        <div className="muted small ellipsis">{fmtDateTime(p.sung_at)}{p.night_title ? ` · ${p.night_title}` : p.practice ? ' · practice' : ''}{p.key_shift ? ` · key ${KEY_LABEL(p.key_shift)}` : ''}{p.cheers ? ` · 👏 ${p.cheers}` : ''}{p.stars ? ` · ★ ${p.stars.toFixed(1)}` : ''}</div>
      </div>
      {p.has_recording && <audio controls preload="none" src={`${V1}/performances/${p.id}/recording`} className="perf-audio" />}
    </div>
  )
}

function CopyCard({ s, possible, onCopy }: { s: Song; possible: boolean; onCopy: (keep: boolean) => void }) {
  return (
    <section className="card pad-card copy-card">
      <div className="row wrap">
        <Icon name="youtube" size={18} />
        <div className="grow min0">
          {s.has_media ? <><b>A copy is kept on this server.</b><div className="muted small">It plays without YouTube, with key change, less voice and recording.</div></>
            : s.copy_state === 'working' || s.copy_state === 'waiting' ? <><b>Keeping a copy… {s.copy_state === 'working' ? `${s.copy_progress}%` : 'waiting'}</b><div className="bar mt-s"><span style={{ width: `${s.copy_progress}%` }} /></div></>
            : s.copy_state === 'failed' ? <><b>The copy did not work.</b><div className="muted small">{s.copy_error}</div></>
            : <><b>Plays in YouTube’s player.</b><div className="muted small">Keep a copy on this server to change the key, take out the voice and record — and to sing without the internet.</div></>}
        </div>
        {s.has_media ? <button type="button" className="btn sm" onClick={() => onCopy(false)}>Drop the copy</button>
          : s.copy_state !== 'working' && s.copy_state !== 'waiting' && <button type="button" className="btn sm" disabled={!possible} title={possible ? undefined : 'This box cannot keep copies (no yt-dlp)'} onClick={() => onCopy(true)}><Icon name="download" size={14} />Keep a copy</button>}
      </div>
    </section>
  )
}

function Details({ s, onSave, reload }: { s: Song; onSave: (p: Partial<Song>) => Promise<boolean>; reload: () => void }) {
  const [f, setF] = useState(s)
  const { toast } = useToast()
  const pic = useRef<HTMLInputElement>(null)
  useEffect(() => { setF(s) }, [s.id, s.updated_at]) // eslint-disable-line react-hooks/exhaustive-deps
  const dirty = (['title', 'artist', 'language', 'kind', 'tags', 'notes', 'key_shift', 'voice_cut', 'video_words'] as const).some(k => f[k] !== s[k])
  return (
    <section className="card pad-card">
      <h3 className="card-h">About the song</h3>
      <div className="two-col tight">
        <Field label="Title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></Field>
        <Field label="Artist"><input value={f.artist} onChange={e => setF({ ...f, artist: e.target.value })} /></Field>
        <Field label="Language"><Select ariaLabel="Language" value={f.language} onChange={v => setF({ ...f, language: v })} options={LANGS} /></Field>
        <Field label="Tags" help="Separated by commas: party, duet, slow…"><input value={f.tags} onChange={e => setF({ ...f, tags: e.target.value })} /></Field>
      </div>
      <Field label="The track"><Segmented size="sm" value={f.kind} onChange={v => setF({ ...f, kind: v })} options={[{ value: 'karaoke', label: 'Karaoke track (no lead voice)' }, { value: 'original', label: 'With the voice' }]} /></Field>
      <div className="two-col tight">
        <Field label={`Usual key: ${KEY_LABEL(f.key_shift)}`} help="Where this song starts when someone sings it."><input type="range" min={-6} max={6} step={1} value={f.key_shift} onChange={e => setF({ ...f, key_shift: Number(e.target.value) })} disabled={!s.has_media && s.source !== 'music'} /></Field>
        <Field label={`Less voice: ${f.voice_cut}%`} help="Turns the lead voice down (it sits in the middle of most songs)."><input type="range" min={0} max={100} step={5} value={f.voice_cut} onChange={e => setF({ ...f, voice_cut: Number(e.target.value) })} disabled={!s.has_media && s.source !== 'music'} /></Field>
      </div>
      {s.is_video && <label className="check-row"><Switch checked={f.video_words} onChange={v => setF({ ...f, video_words: v })} label="The video shows the words" /><span>The video shows the words itself (ours stay hidden)</span></label>}
      <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} placeholder="Starts high; the bridge is fast…" /></Field>
      <div className="row wrap">
        <button type="button" className="btn primary" disabled={!dirty} onClick={async () => { if (await onSave({ title: f.title, artist: f.artist, language: f.language, kind: f.kind, tags: f.tags, notes: f.notes, key_shift: f.key_shift, voice_cut: f.voice_cut, video_words: f.video_words })) toast('Saved') }}>Save</button>
        <button type="button" className="btn" onClick={() => pic.current?.click()}><Icon name="image" size={15} />{s.has_cover ? 'Another picture' : 'Add a picture'}</button>
        <input ref={pic} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={async e => { const file = e.target.files?.[0]; if (!file) return; try { await upload(`${V1}/songs/${s.id}/cover`, 'file', file); reload(); announceChange() } catch (err) { toast(errMsg(err), 'err') } }} />
      </div>
    </section>
  )
}

function BooksCard({ s, books, onChange }: { s: Song; books: Book[]; onChange: () => void }) {
  const { toast } = useToast()
  if (!books.length) return null
  const toggle = async (b: Book, on: boolean) => {
    try { if (on) await api.put(`${V1}/books/${b.id}/songs/${s.id}`); else await api.del(`${V1}/books/${b.id}/songs/${s.id}`); onChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <section className="card pad-card">
      <h3 className="card-h">Song books</h3>
      <div className="chips">{books.map(b => { const on = !!s.books?.includes(b.id); return <button key={b.id} type="button" className={`chip ${on ? 'on' : ''}`} onClick={() => toggle(b, !on)}>{on && <Icon name="check" size={12} />}{b.name}</button> })}</div>
    </section>
  )
}

// ---- the words ----

type WTab = 'words' | 'lookup' | 'time' | 'nudge'

function Lyrics({ s, onSave }: { s: Song; onSave: (p: Partial<Song>) => Promise<boolean> }) {
  const [tab, setTab] = useState<WTab>(s.lyrics ? 'words' : 'lookup')
  const [text, setText] = useState(s.lyrics)
  const [busy, setBusy] = useState(false)
  const { settings } = useApp()
  const { toast } = useToast()
  const appState = useAppState()
  const tr = appState('translate', 'translate:run')
  const [trLines, setTrLines] = useState<string[] | null>(null)
  useEffect(() => { setText(s.lyrics) }, [s.id, s.lyrics])
  const playable = s.has_media || s.source === 'music' || s.source === 'youtube'
  const tidy = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ text: string; note: string }>(`${V1}/ai/tidy`, { text: plainText(text) || text })
      setText(r.text)
      toast(r.note || 'Tidied: look it over, then save')
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const translate = async () => {
    try { const r = await api.get<{ lines: string[] }>(`${V1}/songs/${s.id}/translation?to=${encodeURIComponent(settings?.translate_to || 'en')}`); setTrLines(r.lines) } catch (e) { toast(errMsg(e), 'err') }
  }
  if (s.has_cdg) return <section className="card pad-card"><h3 className="card-h">Words</h3><p className="muted small">This is a CD+G karaoke song: the words come with its graphics.</p></section>
  return (
    <section className="card pad-card">
      <div className="row"><h3 className="card-h grow">Words</h3>{s.lyrics_source && <span className="muted small">from {({ lrclib: 'LRCLIB', music: 'Pocket Music', file: 'the file', typed: 'you', tapped: 'tapping in time', ai: 'Pocket AI', translate: 'Pocket Translate' } as Record<string, string>)[s.lyrics_source] || s.lyrics_source}</span>}</div>
      {s.video_words && <p className="muted small">The video shows the words itself; timed words here are shown only when you switch that off.</p>}
      <Tabs value={tab} onChange={setTab} tabs={[{ id: 'words', label: 'The words', icon: 'text' }, { id: 'lookup', label: 'Look them up', icon: 'search' }, { id: 'time', label: 'Time them', icon: 'timer' }, { id: 'nudge', label: 'Earlier / later', icon: 'clock' }]} />
      {tab === 'words' && <div className="stack">
        <textarea className="lyrics-area" rows={14} value={text} onChange={e => setText(e.target.value)} placeholder={'Paste the words, a line each.\n\nTimed words look like this:\n[00:12.30]First line\n[00:15.80]Second line'} spellCheck={false} />
        <div className="row wrap">
          <button type="button" className="btn primary" disabled={text === s.lyrics} onClick={async () => { if (await onSave({ lyrics: text })) toast('Words saved') }}>Save the words</button>
          {settings?.ai_help && <button type="button" className="btn" disabled={busy || !text.trim()} onClick={tidy} title="Takes out chords, headings and page junk; keeps the words as they are">{busy ? <Spinner /> : <Icon name="sparkles" size={15} />}Tidy with AI</button>}
          {tr.ok && s.lyrics && <button type="button" className="btn" onClick={translate}><Icon name="globe" size={15} />Show a translation</button>}
          <span className="muted small">{parseLyrics(text).synced ? 'Timed' : text.trim() ? 'Not timed yet: use “Time them”' : ''}</span>
        </div>
        {trLines && <div className="tr-preview">{parseLyrics(s.lyrics).lines.map((l, i) => <p key={i}><span>{l.text}</span><span className="muted">{trLines[i]}</span></p>)}</div>}
      </div>}
      {tab === 'lookup' && <Lookup s={s} onUse={async (c: LyricsCandidate) => { if (await onSave({ lyrics: c.lyrics, lyrics_source: c.source } as Partial<Song>)) { toast(c.synced ? 'Timed words saved' : 'Words saved: time them next'); setTab(c.synced ? 'nudge' : 'time') } }} />}
      {tab === 'time' && (playable ? <TapTimer s={s} text={text} onSave={async lrc => { if (await onSave({ lyrics: lrc, lyrics_source: 'tapped' } as Partial<Song>)) { toast('Timed — try it with Sing'); setTab('nudge') } }} /> : <p className="muted small">Add the song file first: the words are timed while it plays.</p>)}
      {tab === 'nudge' && <Nudge s={s} onSave={onSave} />}
    </section>
  )
}

function Lookup({ s, onUse }: { s: Song; onUse: (c: LyricsCandidate) => void }) {
  const [title, setTitle] = useState(s.title)
  const [artist, setArtist] = useState(s.artist)
  const [list, setList] = useState<LyricsCandidate[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [show, setShow] = useState<LyricsCandidate | null>(null)
  const { toast } = useToast()
  const run = async () => {
    setBusy(true)
    try { setList((await api.get<{ candidates: LyricsCandidate[] }>(`${V1}/songs/${s.id}/lyrics/lookup?title=${encodeURIComponent(title)}&artist=${encodeURIComponent(artist)}`)).candidates) } catch (e) { toast(errMsg(e), 'err'); setList([]) } finally { setBusy(false) }
  }
  return (
    <div className="stack">
      <div className="row wrap"><input className="grow" value={title} onChange={e => setTitle(e.target.value)} placeholder="Title" /><input className="grow" value={artist} onChange={e => setArtist(e.target.value)} placeholder="Artist" />
        <button type="button" className="btn primary" disabled={busy || !title.trim()} onClick={run}>{busy ? <Spinner /> : <Icon name="search" size={15} />}Look up</button></div>
      <p className="muted small">Searches <a href="https://lrclib.net" target="_blank" rel="noreferrer">LRCLIB</a>, a free library of timed words{s.source === 'music' ? ', and Pocket Music' : ''}. Only the title and artist are sent.</p>
      {list && !list.length && <p className="muted small">Nothing found. Paste the words in “The words” and time them.</p>}
      <div className="cand-list">{list?.map((c, i) => (
        <div key={i} className="cand">
          <div className="grow min0"><b className="ellipsis block-line">{c.title} <span className="muted">· {c.artist}</span></b><span className="muted small ellipsis block-line">{c.album}{c.duration_ms ? ` · ${fmtLen(c.duration_ms)}` : ''}{s.duration_ms && c.duration_ms && Math.abs(s.duration_ms - c.duration_ms) > 8000 ? ' · a different length: the times may be off' : ''}</span><span className="small ellipsis block-line">{c.preview}</span></div>
          <span className={`badge ${c.synced ? 'ok' : ''}`}>{c.synced ? 'timed' : 'words only'}</span>
          <button type="button" className="btn sm ghost" onClick={() => setShow(c)}>See</button>
          <button type="button" className="btn sm primary" onClick={() => onUse(c)}>Use</button>
        </div>
      ))}</div>
      {show && <Modal title={`${show.title} — ${show.artist}`} onClose={() => setShow(null)} footer={<><button type="button" className="btn" onClick={() => setShow(null)}>Close</button><button type="button" className="btn primary" onClick={() => { onUse(show); setShow(null) }}>Use these</button></>}>
        <pre className="lyrics-pre">{plainText(show.lyrics)}</pre>
      </Modal>}
    </div>
  )
}

/** Time the words: the song plays; press at the start of each line. */
function TapTimer({ s, text, onSave }: { s: Song; text: string; onSave: (lrc: string) => void }) {
  const base = useMemo(() => parseLyrics(text).lines.map(l => l.text), [text])
  const [times, setTimes] = useState<number[]>([])
  const [running, setRunning] = useState(false)
  const stage = useRef<StageHandle>(null)
  const { settings } = useApp()
  const song = useMemo(() => ({ ...stageSong(s), lyrics: '', video_words: true }), [s])
  const tap = useCallback(() => {
    if (!running || !stage.current) return
    setTimes(t => t.length < base.length ? [...t, Math.max(0, Math.round(stage.current!.time() - 150))] : t)
  }, [running, base.length])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.code === 'Space' && running) { e.preventDefault(); tap() } if (e.key === 'Backspace' && running) { e.preventDefault(); setTimes(t => t.slice(0, -1)) } }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [tap, running])
  if (!base.length) return <p className="muted small">Put the words in first (“The words” or “Look them up”).</p>
  const start = async () => { setTimes([]); setRunning(true); stage.current?.seek(0); await stage.current?.play() }
  const i = times.length
  return (
    <div className="stack tap">
      <div className="tap-stage"><Stage ref={stage} song={song} settings={screenSettingsFrom(settings || { count_in: false, lyrics_size: 'm', lyrics_style: 'line', next_line: false, translate_to: '', pitch_line: false, echo_amount: 0 })} onEnded={() => setRunning(false)} /></div>
      <p className="muted small">Press <b>Tap</b> (or the space bar) as each line begins; Backspace takes the last one back. Then save — and move them all a little earlier or later if needed.</p>
      <div className="tap-lines">
        {base.map((l, j) => <div key={j} className={`tap-line ${j === i ? 'cur' : j < i ? 'done' : ''}`}><span className="tnum muted">{times[j] !== undefined ? stamp(times[j]) : ''}</span><span>{l || '·'}</span></div>).slice(Math.max(0, i - 2), i + 4)}
      </div>
      <div className="row wrap">
        {!running ? <button type="button" className="btn primary lg" onClick={start}><Icon name="play" size={16} />{times.length ? 'Start again' : 'Play and time'}</button>
          : <button type="button" className="btn primary lg tap-btn" onClick={tap}>Tap · line {Math.min(i + 1, base.length)} of {base.length}</button>}
        {running && <button type="button" className="btn" onClick={() => { stage.current?.pause(); setRunning(false) }}>Pause</button>}
        <button type="button" className="btn" disabled={times.length < Math.min(2, base.length)} onClick={() => onSave(toLRC(base.map((t, j) => ({ t: times[j] ?? -1, text: t })).filter(l => l.t >= 0)))}>Save {times.length} {times.length === 1 ? 'line' : 'lines'}</button>
      </div>
    </div>
  )
}

function Nudge({ s, onSave }: { s: Song; onSave: (p: Partial<Song>) => Promise<boolean> }) {
  const [off, setOff] = useState(s.lyrics_offset_ms)
  const { toast } = useToast()
  useEffect(() => { setOff(s.lyrics_offset_ms) }, [s.lyrics_offset_ms])
  if (!s.lyrics_synced) return <p className="muted small">The words are not timed yet.</p>
  const step = (d: number) => setOff(o => Math.max(-30000, Math.min(30000, o + d)))
  return (
    <div className="stack">
      <p className="muted small">If the words come too early or too late everywhere, move them all at once. Try it with <Link to={`/sing/${s.id}`}>Sing</Link>.</p>
      <div className="row wrap nudge">
        <button type="button" className="btn" onClick={() => step(-500)}>−0.5 s</button>
        <button type="button" className="btn" onClick={() => step(-100)}>−0.1 s</button>
        <b className="tnum">{off === 0 ? 'As timed' : `${off > 0 ? 'later' : 'earlier'} by ${(Math.abs(off) / 1000).toFixed(1)} s`}</b>
        <button type="button" className="btn" onClick={() => step(100)}>+0.1 s</button>
        <button type="button" className="btn" onClick={() => step(500)}>+0.5 s</button>
        <button type="button" className="btn primary" disabled={off === s.lyrics_offset_ms} onClick={async () => { if (await onSave({ lyrics_offset_ms: off })) toast('Saved') }}>Save</button>
      </div>
    </div>
  )
}
