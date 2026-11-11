// A guest's phone at a karaoke night: no account, just a name. Pick a song
// from the song book (or YouTube, when the host allows it), see your place in
// the queue, cheer for whoever is singing and give them stars.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import type { LiveEntry, LiveMsg, LiveState, YTVideo } from '../lib/types'
import { fmtLen, langName } from '../lib/karaoke'
import { Icon } from '../components/icons'
import { Modal, Segmented, Spinner, useDebounce, useToast } from '../components/ui'

interface Info { title: string; status: string; place: string; starts_at: string; about: string; rules: LiveState['rules']; me?: { id: number; name: string; blocked?: boolean }; live: LiveState }
interface GSong { id: number; title: string; artist: string; language: string; video: boolean; duration_ms: number; duet: boolean }

async function call<T>(url: string, method = 'GET', body?: unknown): Promise<T> {
  const r = await fetch(url, { method, credentials: 'same-origin', headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined })
  if (r.status === 204) return undefined as T
  const j = await r.json().catch(() => ({}))
  if (!r.ok) throw new Error(j.error || r.statusText)
  return j as T
}

export function EnterCode() {
  const [code, setCode] = useState('')
  const navigate = useNavigate()
  return (
    <div className="join-page">
      <div className="join-card">
        <img src="/favicon.svg" alt="" width={56} height={56} />
        <h1>Join a karaoke night</h1>
        <p className="muted">Type the code on the big screen.</p>
        <form className="stack" onSubmit={e => { e.preventDefault(); if (code.trim()) navigate(`/j/${code.trim().toUpperCase()}`) }}>
          <input className="code-input" value={code} onChange={e => setCode(e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 6))} placeholder="SING42" autoFocus autoCapitalize="characters" inputMode="text" />
          <button type="submit" className="btn primary lg block-btn" disabled={code.length < 4}>Join</button>
        </form>
      </div>
    </div>
  )
}

export default function Join() {
  const { code = '' } = useParams()
  const base = `/api/v1/public/j/${code}`
  const [info, setInfo] = useState<Info | null>(null)
  const [err, setErr] = useState('')
  const [live, setLive] = useState<LiveState | null>(null)
  const [tab, setTab] = useState<'songs' | 'queue' | 'cheer'>('songs')
  const [name, setName] = useState(() => localStorage.getItem('pkar-guest-name') || '')
  const [cheer, setCheer] = useState<{ id: number; e: string } | null>(null)
  const { toast } = useToast()

  const load = useCallback(() => call<Info>(base).then(i => { setInfo(i); setLive(i.live) }).catch(e => setErr((e as Error).message)), [base])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    if (!info?.me) return
    let es: EventSource | null = null
    let alive = true
    const open = () => {
      es = new EventSource(`${base}/stream`)
      es.onmessage = ev => {
        const m = JSON.parse(ev.data) as LiveMsg
        if (m.type === 'state') setLive(m)
        if (m.type === 'gone') { setErr('This night is over. Thanks for singing!'); es?.close() }
      }
      es.onerror = () => { es?.close(); if (alive) setTimeout(open, 3000) }
    }
    open()
    return () => { alive = false; es?.close() }
  }, [info?.me?.id, base]) // eslint-disable-line react-hooks/exhaustive-deps

  if (err) return <div className="join-page"><div className="join-card"><Icon name="mic-off" size={36} /><h1>{err}</h1></div></div>
  if (!info) return <div className="join-page"><Spinner size={24} /></div>
  if (!info.me) return (
    <div className="join-page">
      <div className="join-card">
        <div className="join-mic">🎤</div>
        <h1>{info.title}</h1>
        {info.place && <p className="muted">{info.place}</p>}
        {info.about && <p>{info.about}</p>}
        <form className="stack" onSubmit={async e => {
          e.preventDefault()
          try { await call(base, 'POST', { name }); localStorage.setItem('pkar-guest-name', name); load() } catch (x) { toast((x as Error).message, 'err') }
        }}>
          <input value={name} onChange={e => setName(e.target.value)} placeholder="Your name" autoFocus maxLength={40} autoComplete="given-name" />
          <button type="submit" className="btn primary lg block-btn" disabled={!name.trim()}>Join</button>
          <p className="muted small">Your name shows in the queue and on the big screen. No account needed.</p>
        </form>
      </div>
    </div>
  )
  const me = info.me
  const mine = (live?.queue || []).filter(e => e.guest_id === me.id)
  const place = (live?.queue || []).findIndex(e => e.guest_id === me.id)
  const singingNow = live?.current && (live.phase === 'singing' || live.phase === 'paused' || live.phase === 'next') ? live.current : null
  const react = async (emoji: string) => {
    setCheer({ id: Date.now(), e: emoji })
    try { await call(`${base}/react`, 'POST', { emoji }) } catch { /* fine */ }
  }
  return (
    <div className="join-app">
      <header className="join-top">
        <div className="grow min0"><div className="ellipsis"><b>{info.title}</b></div><div className="small muted ellipsis">Hi {me.name} · code {code.toUpperCase()}</div></div>
      </header>
      <NowCard live={live} me={me.id} />
      {mine.length > 0 && tab !== 'queue' && <button type="button" className="join-mine" onClick={() => setTab('queue')}>
        <Icon name="clock" size={15} />{place === 0 ? 'You are next!' : `You are ${place + 1}${['st', 'nd', 'rd'][place] || 'th'} in the queue`} · {mine[0].title}
      </button>}
      <div className="join-tabs"><Segmented value={tab} onChange={setTab} options={[{ value: 'songs', label: 'Pick a song' }, { value: 'queue', label: `Queue${live?.queue.length ? ` (${live.queue.length})` : ''}` }, { value: 'cheer', label: 'Cheer' }]} /></div>
      <main className="join-main">
        {tab === 'songs' && <PickSongs base={base} rules={live?.rules || info.rules} onAsked={() => setTab('queue')} />}
        {tab === 'queue' && <QueueView live={live} me={me.id} base={base} />}
        {tab === 'cheer' && <div className="stack center">
          {singingNow ? <><p className="muted">Cheer for <b>{singingNow.singer}</b> — it shows on the big screen.</p>
            <div className="cheer-grid">{['👏', '🔥', '❤️', '😂', '🎤', '⭐', '🙌', '😮'].map(e => <button key={e} type="button" className={`cheer ${cheer?.e === e ? 'pop' : ''}`} onClick={() => react(e)}>{e}</button>)}</div>
            {singingNow.guest_id !== me.id && <Stars base={base} entry={singingNow} />}</>
            : live?.phase === 'score' && live.result ? <ScoreCard live={live} base={base} me={me.id} /> : <p className="muted">Nobody is singing right now.</p>}
        </div>}
      </main>
    </div>
  )
}

function NowCard({ live, me }: { live: LiveState | null; me: number }) {
  if (!live) return null
  const c = live.current
  if (live.status !== 'live') return <div className="join-now muted">The night has not started yet — pick your songs already.</div>
  if (!c) return <div className="join-now muted">The microphone is free: ask for a song!</div>
  const label = live.phase === 'next' ? 'Up next' : live.phase === 'score' ? 'Just sang' : 'Singing now'
  return (
    <div className={`join-now ${c.guest_id === me ? 'me' : ''}`}>
      <span className="small muted">{label}</span>
      <div><b>{c.singer}</b> · {c.title}</div>
      {c.guest_id === me && live.phase === 'next' && <div className="join-you">🎤 It’s you — come to the microphone!</div>}
    </div>
  )
}

function PickSongs({ base, rules, onAsked }: { base: string; rules: LiveState['rules']; onAsked: () => void }) {
  const [mode, setMode] = useState<'book' | 'youtube'>('book')
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 300)
  const [songs, setSongs] = useState<GSong[] | null>(null)
  const [videos, setVideos] = useState<YTVideo[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [pick, setPick] = useState<{ song?: GSong; video?: YTVideo } | null>(null)
  useEffect(() => {
    if (mode !== 'book') return
    call<{ songs: GSong[] }>(`${base}/songs?q=${encodeURIComponent(dq)}`).then(r => setSongs(r.songs)).catch(() => setSongs([]))
  }, [dq, mode, base])
  useEffect(() => {
    if (mode !== 'youtube') return
    if (!dq) { setVideos(null); return }
    setBusy(true)
    call<{ videos: YTVideo[] }>(`${base}/youtube?q=${encodeURIComponent(dq)}`).then(r => setVideos(r.videos)).catch(() => setVideos([])).finally(() => setBusy(false))
  }, [dq, mode, base])
  return (
    <div className="stack">
      {rules.youtube && <Segmented size="sm" value={mode} onChange={setMode} options={[{ value: 'book', label: rules.book ? 'Tonight’s song book' : 'The song list' }, { value: 'youtube', label: 'Find on YouTube' }]} />}
      <div className="search-box"><Icon name="search" size={16} /><input value={q} onChange={e => setQ(e.target.value)} placeholder={mode === 'book' ? 'A song or an artist' : 'Search YouTube for karaoke'} />{busy && <Spinner />}</div>
      {mode === 'book' && <div className="g-list">
        {songs?.map(s => <button key={s.id} type="button" className="g-row" onClick={() => setPick({ song: s })}>
          <Icon name={s.video ? 'video' : 'music'} size={16} />
          <span className="grow min0"><span className="ellipsis block-line">{s.title}</span><span className="muted small ellipsis block-line">{s.artist}{s.language ? ` · ${langName(s.language)}` : ''}{s.duration_ms ? ` · ${fmtLen(s.duration_ms)}` : ''}</span></span>
          {s.duet && <span className="badge">duet</span>}<Icon name="plus" size={16} />
        </button>)}
        {songs && !songs.length && <p className="muted small">Nothing like that in the song list.{rules.youtube ? ' Try “Find on YouTube”.' : ''}</p>}
      </div>}
      {mode === 'youtube' && <div className="g-list">
        {videos?.map(v => <button key={v.id} type="button" className="g-row" onClick={() => setPick({ video: v })}>
          <img src={v.thumb} alt="" className="g-thumb" loading="lazy" />
          <span className="grow min0"><span className="ellipsis2">{v.title}</span><span className="muted small ellipsis block-line">{v.channel}{v.duration_ms ? ` · ${fmtLen(v.duration_ms)}` : ''}</span></span>
          <Icon name="plus" size={16} />
        </button>)}
        {!dq && <p className="muted small">Search for the song with “karaoke” — videos with the words on screen work best.</p>}
      </div>}
      {pick && <AskModal base={base} pick={pick} approve={rules.approve} onClose={() => setPick(null)} onDone={() => { setPick(null); onAsked() }} />}
    </div>
  )
}

function AskModal({ base, pick, approve, onClose, onDone }: { base: string; pick: { song?: GSong; video?: YTVideo }; approve: boolean; onClose: () => void; onDone: () => void }) {
  const [withWho, setWith] = useState('')
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const title = pick.song?.title || pick.video?.title || ''
  const go = async () => {
    setBusy(true)
    try {
      if (pick.song) await call(`${base}/requests`, 'POST', { song_id: pick.song.id, with: withWho })
      else await call(`${base}/youtube`, 'POST', { ...pick.video, with: withWho })
      toast(approve || pick.video ? 'Asked! The host puts it in the queue' : 'You are in the queue!')
      onDone()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Sing this?" onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Back</button><button type="button" className="btn primary" disabled={busy} onClick={go}>{busy ? <Spinner /> : <Icon name="mic" size={15} />}{approve ? 'Ask for it' : 'Put me in the queue'}</button></>}>
      <p><b>{title}</b>{pick.song?.artist ? ` — ${pick.song.artist}` : pick.video?.channel ? ` · ${pick.video.channel}` : ''}</p>
      <input value={withWho} onChange={e => setWith(e.target.value)} placeholder="Singing with someone? Their name (optional)" maxLength={40} />
      {approve && <p className="muted small">The host says yes to each song tonight.</p>}
    </Modal>
  )
}

function QueueView({ live, me, base }: { live: LiveState | null; me: number; base: string }) {
  const { toast } = useToast()
  if (!live) return <Spinner />
  const cancel = async (e: LiveEntry) => { try { await call(`${base}/requests/${e.id}`, 'DELETE') } catch (x) { toast((x as Error).message, 'err') } }
  return (
    <div className="stack">
      {live.queue.length === 0 && <p className="muted">The queue is empty — pick a song!</p>}
      <ol className="g-queue">
        {live.queue.map((e, i) => <li key={e.id} className={e.guest_id === me ? 'me' : ''}>
          <span className="g-n">{i + 1}</span>
          <span className="grow min0"><span className="ellipsis block-line"><b>{e.singer}</b></span><span className="muted small ellipsis block-line">{e.title}</span></span>
          {e.guest_id === me && <button type="button" className="btn sm ghost" onClick={() => cancel(e)}>Take out</button>}
        </li>)}
      </ol>
      {live.rules.per_guest > 0 && <p className="muted small">Up to {live.rules.per_guest} {live.rules.per_guest === 1 ? 'song' : 'songs'} each in the queue; everyone gets a turn before anyone sings twice.</p>}
    </div>
  )
}

function Stars({ base, entry }: { base: string; entry: LiveEntry }) {
  const [stars, setStars] = useState(0)
  const { toast } = useToast()
  useEffect(() => { setStars(0) }, [entry.id])
  const vote = async (n: number) => { setStars(n); try { await call(`${base}/vote`, 'POST', { entry_id: entry.id, stars: n }) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack center">
      <p className="muted small">How is it going?</p>
      <div className="star-pick">{[1, 2, 3, 4, 5].map(n => <button key={n} type="button" className={n <= stars ? 'on' : ''} onClick={() => vote(n)} aria-label={`${n} stars`}>★</button>)}</div>
    </div>
  )
}

function ScoreCard({ live, base, me }: { live: LiveState; base: string; me: number }) {
  const r = live.result!
  const entry = live.current
  return (
    <div className="stack center">
      <p className="muted">{r.singer} sang {r.title}</p>
      {r.score !== null && <div className="g-score">{r.score}<small> points</small></div>}
      {entry && entry.guest_id !== me && <Stars base={base} entry={entry} />}
    </div>
  )
}
