// The big screen of a karaoke night (a TV, a projector, the laptop on the
// table): no sign-in, its own link. It plays the queue — "up next", a
// countdown, the song with its words, the score — shows the code guests join
// with, cheers from their phones, and photos between songs.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import type { LiveMsg, LiveState } from '../lib/types'
import { QRCode } from '../components/QRCode'
import { Stage, type Reaction, type StageHandle, type StageResult } from '../components/Stage'
import { Icon } from '../components/icons'

const MIC_KEY = 'pkar-screen-mic'

export default function Screen() {
  const { key } = useParams()
  const base = `/api/v1/public/screen/${key}`
  const [live, setLive] = useState<LiveState | null>(null)
  const [gone, setGone] = useState(false)
  const [armed, setArmed] = useState(false) // a press on the page lets sound play
  const [reacts, setReacts] = useState<Reaction[]>([])
  const [photos, setPhotos] = useState<string[]>([])
  const [photoAt, setPhotoAt] = useState(0)
  const [now, setNow] = useState(Date.now())
  const [mic, setMic] = useState(() => localStorage.getItem(MIC_KEY) === '1')
  const [micErr, setMicErr] = useState('')
  const [translation, setTranslation] = useState<string[] | undefined>()
  const [problem, setProblem] = useState('')
  const stage = useRef<StageHandle>(null)
  const liveRef = useRef<LiveState | null>(null)
  liveRef.current = live
  const skew = useRef(0)

  const report = useCallback((action: string, extra: Record<string, unknown> = {}) => {
    fetch(`${base}/report`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action, ...extra }) }).catch(() => {})
  }, [base])

  // the night, then its stream
  useEffect(() => {
    let es: EventSource | null = null
    let alive = true
    fetch(base).then(r => r.ok ? r.json() : Promise.reject()).then((s: LiveState) => { if (alive) { setLive(s); skew.current = Date.parse(s.now) - Date.now() } }).catch(() => setGone(true))
    const open = () => {
      es = new EventSource(`${base}/stream`)
      es.onmessage = e => {
        const m = JSON.parse(e.data) as LiveMsg
        if (m.type === 'state') { setLive(m); skew.current = Date.parse(m.now) - Date.now() }
        else if (m.type === 'gone') { setGone(true); es?.close() }
        else if (m.type === 'react') addReact(m.emoji, m.name)
        else if (m.type === 'cmd') {
          const s = stage.current
          if (!s) return
          if (m.cmd === 'play') s.play()
          if (m.cmd === 'pause') s.pause()
          if (m.cmd === 'seek') s.seek(m.value)
          if (m.cmd === 'stop') s.stop()
        }
      }
      es.onerror = () => { es?.close(); if (alive) setTimeout(open, 3000) }
    }
    open()
    return () => { alive = false; es?.close() }
  }, [base])

  const addReact = (emoji: string, name: string) => {
    const id = Date.now() + Math.random()
    setReacts(r => [...r.slice(-24), { id, emoji, name, x: 8 + Math.random() * 84 }])
    setTimeout(() => setReacts(r => r.filter(x => x.id !== id)), 3600)
  }

  // pictures from Pocket Photos between songs
  useEffect(() => {
    if (!live?.settings?.photos) return
    fetch(`${base}/photos`).then(r => r.json()).then(r => setPhotos(r.photos || [])).catch(() => {})
  }, [live?.settings?.photos, base])
  useEffect(() => {
    const t = setInterval(() => { setNow(Date.now()); }, 250)
    const p = setInterval(() => setPhotoAt(i => i + 1), 9000)
    return () => { clearInterval(t); clearInterval(p) }
  }, [])

  // the words in another language, when the host wants them
  const song = live?.song
  useEffect(() => {
    setTranslation(undefined)
    const to = live?.settings?.translate_to
    if (!song || !to || !song.lyrics || song.language === to) return
    fetch(`${base}/songs/${song.id}/translation?to=${encodeURIComponent(to)}`).then(r => r.json()).then(r => { if (r.lines?.length) setTranslation(r.lines) }).catch(() => {})
  }, [song?.id, live?.settings?.translate_to, base]) // eslint-disable-line react-hooks/exhaustive-deps

  // the countdown before the next song starts by itself
  const phase = live?.phase
  const phaseAt = live?.phase_at ? Date.parse(live.phase_at) - skew.current : 0
  const left = live && phase === 'next' && live.autoplay > 0 ? Math.max(0, Math.ceil((phaseAt + live.autoplay * 1000 - now) / 1000)) : -1
  const startedFor = useRef(0)
  useEffect(() => {
    if (!armed || !live?.current || phase !== 'next' || left !== 0 || startedFor.current === live.current.id) return
    startedFor.current = live.current.id
    setTimeout(() => stage.current?.play(), 200)
  }, [left, phase, armed, live?.current?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  // the score stays on for ten seconds
  useEffect(() => {
    if (phase !== 'score') return
    const t = setTimeout(() => report('advance'), 10000)
    return () => clearTimeout(t)
  }, [phase, live?.current?.id, report])

  // the microphone of this screen: a score and the pitch line
  useEffect(() => {
    if (!armed || !stage.current || phase !== 'singing') return
    stage.current.mic(mic, false, 0).then(err => setMicErr(err))
  }, [mic, armed, phase, live?.current?.id])

  // keys on a laptop: space plays or pauses, N calls the next singer
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const s = liveRef.current
      if (!s) return
      if (e.code === 'Space') { e.preventDefault(); if (s.phase === 'next' || s.phase === 'paused') stage.current?.play(); else if (s.phase === 'singing') { stage.current?.pause(); report('pause') } }
      if (e.key === 'n' || e.key === 'N') report('next')
      if (e.key === 'f' || e.key === 'F') document.documentElement.requestFullscreen?.().catch(() => {})
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [report])

  const onStarted = () => report('started')
  const onEnded = (r: StageResult) => {
    const s = liveRef.current
    report('finished', { entry_id: s?.current?.id || 0, score: s?.scores ? r.score : null, on_time: r.on_time, steady: r.steady, sung_ms: r.sung_ms })
  }
  const onTime = (ms: number, playing: boolean) => { if (liveRef.current?.phase === 'singing' || liveRef.current?.phase === 'paused') report('progress', { value: Math.round(ms), playing }) }
  const onError = (m: string) => {
    setProblem(m)
    const id = liveRef.current?.current?.id
    setTimeout(() => { setProblem(''); if (liveRef.current?.current?.id === id) report('finished', { entry_id: id || 0, sung_ms: 0 }) }, 6000)
  }

  const settings = useMemo(() => live?.settings || { count_in: true, lyrics_size: 'l' as const, lyrics_style: 'sweep' as const, next_line: true, translate_to: '', pitch_line: true, echo_amount: 0, photos: false }, [live?.settings])

  if (gone) return <div className="screen center-msg"><Icon name="tv" size={40} /><h1>This screen link is not in use any more</h1><p>Open the big screen again from the night’s page.</p></div>
  if (!live) return <div className="screen center-msg"><div className="spinner" /></div>
  const joinCode = live.join_code
  const showStage = !!live.current && !!live.song && (phase === 'next' || phase === 'singing' || phase === 'paused' || phase === 'score')
  const bg = photos.length ? photos[photoAt % photos.length] : ''
  const qrSmall = <div className="scr-join-small"><QRCode value={live.join_url} size={92} /><div><div className="scr-code">{joinCode}</div><div className="op7 small">{live.join_url.replace(/^https?:\/\//, '').replace(/\/j\/.*/, '')}/j</div></div></div>

  return (
    <div className="screen" onClick={() => { if (!armed) setArmed(true) }}>
      {showStage && live.song && <Stage key={`${live.current!.id}-${live.song.id}`} ref={stage} big song={live.song} settings={settings} singer={live.current!.singer} translation={translation}
        reactions={reacts} keyShift={live.key_now} voiceCut={live.voice_now} volume={live.volume} onStarted={onStarted} onEnded={onEnded} onTime={onTime} onError={onError} />}

      {!showStage && <div className="scr-idle">
        {bg && <img className="scr-photo" src={bg} alt="" key={bg} />}
        <div className="scr-idle-card">
          <div className="scr-mic">🎤</div>
          <h1>{live.title}</h1>
          {live.status !== 'live' ? <p className="op7">The night has not started yet.</p> : <p className="op7">Pick your song on your phone:</p>}
          <div className="scr-join"><QRCode value={live.join_url} size={220} /><div><div className="op7">Scan, or go to</div><div className="scr-url">{live.join_url.replace(/^https?:\/\//, '')}</div><div className="op7 mt">and type the code</div><div className="scr-code big">{joinCode}</div></div></div>
          {!!live.queue.length && <div className="scr-upnext"><div className="op7">Up next</div>{live.queue.slice(0, 4).map(e => <div key={e.id} className="scr-q"><b>{e.singer}</b> · {e.title}</div>)}</div>}
        </div>
        <div className="stage-reacts" aria-hidden>{reacts.map(r => <span key={r.id} className="react" style={{ left: `${r.x}%` }}>{r.emoji}<small>{r.name}</small></span>)}</div>
      </div>}

      {showStage && phase === 'next' && <div className="scr-next">
        <div className="op7">Up next</div>
        <div className="scr-next-who">🎤 {live.current!.singer}</div>
        <div className="scr-next-song">{live.song!.title}{live.song!.artist ? ` — ${live.song!.artist}` : ''}</div>
        {left > 0 ? <div className="scr-count">{left}</div> : live.autoplay === 0 || !armed ? <div className="op7">{armed ? 'Starts when the host presses play' : 'Click anywhere to let the screen play sound'}</div> : null}
        {!armed && <button type="button" className="btn primary lg" onClick={() => { setArmed(true); stage.current?.play() }}><Icon name="play" size={18} />Start the song</button>}
      </div>}

      {phase === 'score' && live.result && <div className="scr-score">
        <div className="op7">{live.result.singer} · {live.result.title}</div>
        {live.result.score !== null ? <div className="scr-points">{live.result.score}<small>points</small></div> : <div className="scr-points sm">👏</div>}
        <div className="row center wrap scr-score-meta">
          {live.result.best && live.result.score !== null && <span className="badge ok">Best of the night so far</span>}
          {live.result.cheers > 0 && <span>👏 {live.result.cheers}</span>}
          {live.result.stars !== null && <span>★ {live.result.stars.toFixed(1)} from {live.result.votes}</span>}
        </div>
        {live.queue[0] && <div className="op7 mt">Next: <b>{live.queue[0].singer}</b> · {live.queue[0].title}</div>}
      </div>}

      {showStage && phase !== 'score' && <div className="scr-corner">{qrSmall}</div>}
      {showStage && phase === 'singing' && <div className="scr-singer">🎤 {live.current!.singer}{live.cheers ? <span className="op7"> · 👏 {live.cheers}</span> : null}</div>}
      {!armed && phase !== 'next' && <div className="scr-arm">Click anywhere to let this screen play sound · F for full screen</div>}
      {problem && <div className="scr-problem"><Icon name="alert" size={18} />{problem} Moving on…</div>}
      <div className="scr-tools">
        <button type="button" className={`btn ghost icon-only on-dark ${mic ? 'lit' : ''}`} title={mic ? 'This screen’s microphone gives a score' : 'Use this screen’s microphone for a score and the pitch line'} aria-label="Microphone" onClick={e => { e.stopPropagation(); const v = !mic; setMic(v); localStorage.setItem(MIC_KEY, v ? '1' : '0'); setArmed(true) }}><Icon name={mic ? 'mic' : 'mic-off'} size={18} /></button>
        <button type="button" className="btn ghost icon-only on-dark" title="Full screen (F)" aria-label="Full screen" onClick={e => { e.stopPropagation(); document.documentElement.requestFullscreen?.().catch(() => {}) }}><Icon name="maximize" size={18} /></button>
        {micErr && <span className="small op7">{micErr}</span>}
      </div>
    </div>
  )
}
