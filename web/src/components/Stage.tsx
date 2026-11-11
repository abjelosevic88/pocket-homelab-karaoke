// The singing view: the song (a video, a YouTube video, CD+G graphics or the
// cover over moving light) with the words filling as they are sung, a
// count-in before the first line, the voice's notes along the bottom and the
// reactions from the room. Used by the Sing page and by the big screen.
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import type { LiveSong, ScreenSettings } from '../lib/types'
import { lineAt, lineProgress, parseLyrics, type Line } from '../lib/lyrics'
import { MicInput, Scorer, SongEngine, noteName } from '../lib/audio'
import { CDGPlayer } from '../lib/cdg'
import { loadYouTube, type YTPlayer } from '../lib/youtube'

export interface StageResult { score: number | null; on_time: number | null; steady: number | null; sung_ms: number; recording?: Blob }

export interface StageHandle {
  play: () => Promise<void>
  pause: () => void
  seek: (ms: number) => void
  stop: () => void
  time: () => number
  playing: () => boolean
  setKey: (k: number) => void
  setVoice: (v: number) => void
  setVolume: (v: number) => void
  mic: (on: boolean, monitor: boolean, echo: number) => Promise<string>
  record: (on: boolean) => void
  /** what is possible with this song */
  can: { key: boolean; voice: boolean; record: boolean }
}

export interface Reaction { id: number; emoji: string; name: string; x: number }

interface Props {
  song: LiveSong
  settings: ScreenSettings
  singer?: string
  big?: boolean
  translation?: string[]
  reactions?: Reaction[]
  keyShift?: number
  voiceCut?: number
  volume?: number
  onTime?: (ms: number, playing: boolean) => void
  onStarted?: () => void
  onEnded?: (r: StageResult) => void
  onError?: (msg: string) => void
}

const SIZES = { m: 1, l: 1.25, xl: 1.55 }

export const Stage = forwardRef<StageHandle, Props>(function Stage({ song, settings, singer, big, translation, reactions = [], keyShift = 0, voiceCut = 0, volume = 100, onTime, onStarted, onEnded, onError }, ref) {
  const mediaRef = useRef<HTMLVideoElement | HTMLAudioElement | null>(null)
  const ytBox = useRef<HTMLDivElement | null>(null)
  const yt = useRef<YTPlayer | null>(null)
  const cdgCanvas = useRef<HTMLCanvasElement | null>(null)
  const cdg = useRef<CDGPlayer | null>(null)
  const engine = useRef<SongEngine | null>(null)
  const mic = useRef<MicInput | null>(null)
  const scorer = useRef(new Scorer())
  const recorder = useRef<ReturnType<SongEngine['record']>>(null)
  const recording = useRef(false)
  const started = useRef(false)
  const ended = useRef(false)
  const sungFrom = useRef(0)
  const pitchCanvas = useRef<HTMLCanvasElement | null>(null)
  const [t, setT] = useState(0)
  const [isPlaying, setPlaying] = useState(false)
  const [note, setNote] = useState<number | null>(null)
  const [ytReady, setYtReady] = useState(false)
  const [micOn, setMicOn] = useState(false)
  const cb = useRef({ onTime, onStarted, onEnded, onError })
  cb.current = { onTime, onStarted, onEnded, onError }

  const { lines, synced } = useMemo(() => parseLyrics(song.lyrics), [song.lyrics])
  const showWords = !song.video_words && !song.cdg && lines.length > 0
  const offset = song.offset_ms || 0
  const isYT = !!song.youtube && !song.media
  const isVideo = !!song.media && song.is_video
  const can = { key: !!song.media, voice: !!song.media, record: !!song.media }

  // ---- the media ----
  const ensureEngine = useCallback(async () => {
    if (!song.media || !mediaRef.current) return null
    if (!engine.current) {
      try { engine.current = new SongEngine(mediaRef.current) } catch { return null }
    }
    await engine.current.ready()
    engine.current.setKey(keyShift)
    engine.current.setVoiceCut(voiceCut)
    engine.current.setVolume(volume)
    return engine.current
  }, [song.media]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { engine.current?.setKey(keyShift) }, [keyShift])
  useEffect(() => { engine.current?.setVoiceCut(voiceCut) }, [voiceCut])
  useEffect(() => { engine.current?.setVolume(volume); if (yt.current) yt.current.setVolume(volume); if (mediaRef.current && !engine.current) mediaRef.current.volume = volume / 100 }, [volume])

  // YouTube's own player
  useEffect(() => {
    if (!isYT || !ytBox.current) return
    let alive = true
    const box = document.createElement('div')
    ytBox.current.appendChild(box)
    loadYouTube().then(YT => {
      if (!alive) return
      yt.current = new YT.Player(box, {
        videoId: song.youtube!, host: 'https://www.youtube-nocookie.com',
        playerVars: { autoplay: 0, controls: 0, disablekb: 1, modestbranding: 1, rel: 0, playsinline: 1, iv_load_policy: 3, fs: 0 },
        events: {
          onReady: () => { if (alive) { setYtReady(true); yt.current?.setVolume(volume) } },
          onStateChange: (e: { data: number }) => {
            if (e.data === 1) { setPlaying(true); if (!started.current) { started.current = true; sungFrom.current = performance.now(); cb.current.onStarted?.() } }
            if (e.data === 2) setPlaying(false)
            if (e.data === 0) finish()
          },
          onError: (e: { data: number }) => cb.current.onError?.(e.data === 101 || e.data === 150 ? 'This video may only be played on YouTube itself.' : 'YouTube could not play this video.'),
        },
      })
    }).catch(() => cb.current.onError?.('YouTube’s player did not load.'))
    return () => { alive = false; try { yt.current?.destroy() } catch { /* gone */ } yt.current = null; setYtReady(false); box.remove() }
  }, [song.youtube, isYT]) // eslint-disable-line react-hooks/exhaustive-deps

  // CD+G graphics
  useEffect(() => {
    cdg.current = null
    if (!song.cdg || !cdgCanvas.current) return
    let alive = true
    fetch(song.cdg, { credentials: 'same-origin' }).then(r => r.arrayBuffer()).then(b => { if (alive && cdgCanvas.current) cdg.current = new CDGPlayer(b, cdgCanvas.current) }).catch(() => {})
    return () => { alive = false }
  }, [song.cdg])

  const now = useCallback((): number => {
    if (isYT) return (yt.current?.getCurrentTime?.() || 0) * 1000
    return (mediaRef.current?.currentTime || 0) * 1000
  }, [isYT])

  const finish = useCallback(async () => {
    if (ended.current) return
    ended.current = true
    setPlaying(false)
    const res = scorer.current.result()
    let rec: Blob | undefined
    if (recorder.current && recording.current) { rec = await recorder.current.stop(); recording.current = false }
    cb.current.onEnded?.({ score: res?.score ?? null, on_time: res?.on_time ?? null, steady: res?.steady ?? null, sung_ms: started.current ? Math.round(performance.now() - sungFrom.current) : 0, recording: rec && rec.size > 2000 ? rec : undefined })
  }, [])

  // every frame: the time, the voice's note, the CD+G picture
  useEffect(() => {
    let raf = 0, last = 0, lastReport = 0
    const loop = (ts: number) => {
      raf = requestAnimationFrame(loop)
      const ms = now()
      if (cdg.current) cdg.current.render(ms)
      if (ts - last < 33) return
      last = ts
      setT(ms)
      const playingNow = isYT ? isPlaying : !!mediaRef.current && !mediaRef.current.paused
      if (mic.current && playingNow) {
        const f = mic.current.frame(ms)
        const li = lineAt(lines, ms - offset)
        const singing = synced ? (li >= 0 && lines[li].text !== '' && ms - offset < lines[li].end + 400) : ms > 4000
        scorer.current.add(f, singing)
        setNote(f.midi)
        drawPitch()
      }
      if (ts - lastReport > 1000) { lastReport = ts; cb.current.onTime?.(ms, playingNow) }
    }
    raf = requestAnimationFrame(loop)
    return () => cancelAnimationFrame(raf)
  }, [now, lines, synced, offset, isYT, isPlaying]) // eslint-disable-line react-hooks/exhaustive-deps

  const drawPitch = () => {
    const c = pitchCanvas.current
    if (!c) return
    const w = c.clientWidth, h = c.clientHeight
    if (!w || !h) return
    const dpr = window.devicePixelRatio || 1
    if (c.width !== Math.round(w * dpr)) { c.width = Math.round(w * dpr); c.height = Math.round(h * dpr) }
    const g = c.getContext('2d')!
    g.setTransform(dpr, 0, 0, dpr, 0, 0)
    g.clearRect(0, 0, w, h)
    const trail = scorer.current.trail
    if (!trail.length) return
    const tEnd = trail[trail.length - 1].t
    const lo = 40, hi = 84
    g.lineWidth = 4
    g.lineCap = 'round'
    g.strokeStyle = 'rgba(255,255,255,.85)'
    g.beginPath()
    let pen = false
    for (const f of trail) {
      const x = w - ((tEnd - f.t) / 6000) * w
      if (f.midi === null) { pen = false; continue }
      const m = ((f.midi - 12 * Math.floor((f.midi - lo) / 12 / 2)) - lo)
      const y = h - Math.max(0, Math.min(1, m / (hi - lo))) * (h - 8) - 4
      if (!pen) { g.moveTo(x, y); pen = true } else g.lineTo(x, y)
    }
    g.stroke()
  }

  // a new song: back to the start
  useEffect(() => {
    started.current = false
    ended.current = false
    scorer.current = new Scorer()
    setT(0)
    setPlaying(false)
  }, [song.id])

  useEffect(() => () => { engine.current?.close(); engine.current = null; mic.current = null }, [])

  useImperativeHandle(ref, () => ({
    can,
    play: async () => {
      if (isYT) { yt.current?.playVideo(); return }
      const el = mediaRef.current
      if (!el) return
      await ensureEngine()
      try { await el.play() } catch (e) { cb.current.onError?.((e as Error).message || 'The song could not start.') }
    },
    pause: () => { if (isYT) yt.current?.pauseVideo(); else mediaRef.current?.pause() },
    seek: (ms: number) => {
      if (isYT) yt.current?.seekTo(ms / 1000, true)
      else if (mediaRef.current) mediaRef.current.currentTime = ms / 1000
      setT(ms)
    },
    stop: () => { if (isYT) yt.current?.stopVideo(); else if (mediaRef.current) { mediaRef.current.pause(); mediaRef.current.currentTime = 0 } ended.current = true },
    time: now,
    playing: () => isYT ? isPlaying : !!mediaRef.current && !mediaRef.current.paused,
    setKey: (k: number) => engine.current?.setKey(k),
    setVoice: (v: number) => engine.current?.setVoiceCut(v),
    setVolume: (v: number) => engine.current?.setVolume(v),
    mic: async (on: boolean, monitor: boolean, echo: number) => {
      if (!on) { engine.current?.stopMic(); mic.current?.close(); mic.current = null; setNote(null); setMicOn(false); return '' }
      try {
        const e = await ensureEngine()
        if (e) mic.current = await e.startMic(monitor, echo)
        else {
          // a YouTube video: only listen (the video's own sound is out of reach)
          if (!mic.current) {
            const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
            mic.current = await MicInput.open(new AC(), monitor, echo)
          } else mic.current.setMonitor(monitor, echo)
        }
        setMicOn(true)
        return ''
      } catch (err) { return (err as Error).name === 'NotAllowedError' ? 'The browser was not allowed to use the microphone.' : 'No microphone found.' }
    },
    record: (on: boolean) => {
      if (!engine.current) return
      if (on && !recording.current) { recorder.current = engine.current.record(); recorder.current?.start(); recording.current = !!recorder.current }
      if (!on && recording.current) { recorder.current?.stop(); recording.current = false }
    },
  }), [isYT, ensureEngine, now, isPlaying]) // eslint-disable-line react-hooks/exhaustive-deps

  const onMediaPlay = () => {
    setPlaying(true)
    if (!started.current) { started.current = true; sungFrom.current = performance.now(); cb.current.onStarted?.() }
  }

  // ---- the words ----
  const st = t - offset
  const cur = showWords ? lineAt(lines, st) : -1
  const firstAt = lines[0]?.t ?? 0
  const before = synced && (cur < 0 || st < firstAt)
  const countdown = settings.count_in && synced && before && firstAt - st <= 3600 && firstAt - st > 0 ? Math.ceil((firstAt - st) / 1200) : 0
  const gapNext = synced && cur >= 0 && lines[cur + 1] && st > lines[cur].end + 500 && lines[cur + 1].t - st > 4000
  const scale = SIZES[settings.lyrics_size] || 1.25

  let view: { line: Line | null; next: Line | null; idx: number }
  if (!synced) view = { line: null, next: null, idx: -1 }
  else if (before) view = { line: null, next: lines[0], idx: -1 }
  else view = { line: gapNext ? null : lines[cur], next: lines[cur + 1] || null, idx: cur }

  const durMs = song.duration_ms || 0
  const pct = durMs ? Math.min(100, (t / durMs) * 100) : 0
  const title = !isPlaying && t < 500
  const res = scorer.current

  return (
    <div className={`stage ${big ? 'big' : ''} ${isVideo || isYT || song.cdg ? 'has-picture' : 'no-picture'}`} style={{ ['--lyr-scale' as string]: String(scale) }}>
      <div className="stage-bg">
        {!isVideo && !isYT && !song.cdg && <div className="stage-glow" style={song.cover ? { backgroundImage: `url("${song.cover}")` } : undefined} />}
        {song.media && (isVideo
          ? <video ref={el => { mediaRef.current = el }} src={song.media} playsInline preload="auto" onPlay={onMediaPlay} onPause={() => setPlaying(false)} onEnded={finish} onError={() => cb.current.onError?.('The song file could not be played.')} />
          : <audio ref={el => { mediaRef.current = el }} src={song.media} preload="auto" onPlay={onMediaPlay} onPause={() => setPlaying(false)} onEnded={finish} onError={() => cb.current.onError?.('The song file could not be played.')} />)}
        {isYT && <div className="stage-yt" ref={ytBox} />}
        {song.cdg && <canvas ref={cdgCanvas} className="stage-cdg" />}
        {!isVideo && !isYT && !song.cdg && song.cover && <img className="stage-cover" src={song.cover} alt="" />}
      </div>

      {title && <div className="stage-title">
        <div className="stage-title-song">{song.title}</div>
        {song.artist && <div className="stage-title-artist">{song.artist}</div>}
        {singer && <div className="stage-title-singer">🎤 {singer}</div>}
        {isYT && !ytReady && <div className="stage-title-note">Loading YouTube…</div>}
      </div>}

      {showWords && !title && <div className="stage-words" aria-live="off">
        {!synced && <div className="lyr-plain">{lines.map((l, i) => <p key={i}>{l.text || ' '}</p>)}</div>}
        {synced && <>
          {countdown > 0 && <div className="lyr-count" aria-label={`${countdown}`}>{[3, 2, 1].map(n => <span key={n} className={n <= countdown ? 'on' : ''} />)}</div>}
          <div className={`lyr-line now ${settings.lyrics_style}`}>{view.line ? <SungLine line={view.line} t={st} style={settings.lyrics_style} /> : <span className="lyr-rest">{before ? '' : '♪'}</span>}</div>
          {view.line && translation?.[view.idx] && <div className="lyr-tr">{translation[view.idx]}</div>}
          {settings.next_line && view.next && <div className="lyr-line next">{view.next.text}</div>}
        </>}
      </div>}

      {micOn && settings.pitch_line && <div className="stage-pitch">
        <canvas ref={pitchCanvas} />
        <span className="stage-note">{note !== null ? noteName(note) : '·'}</span>
      </div>}
      {micOn && res.heard && <div className="stage-live-score" title="Singing heard">🎤</div>}

      <div className="stage-reacts" aria-hidden>
        {reactions.map(r => <span key={r.id} className="react" style={{ left: `${r.x}%` }}>{r.emoji}<small>{r.name}</small></span>)}
      </div>

      <div className="stage-progress" aria-hidden><span style={{ width: `${pct}%` }} /></div>
    </div>
  )
})

/** One line, the sung part filled in word by word. */
function SungLine({ line, t, style }: { line: Line; t: number; style: 'sweep' | 'line' }) {
  if (style === 'line') return <span className="lyr-text">{line.text}</span>
  const p = lineProgress(line, t)
  const words = line.text.split(/(\s+)/)
  const total = line.text.length || 1
  let pos = 0
  return <span className="lyr-text">{words.map((w, i) => {
    const start = pos / total
    pos += w.length
    const end = pos / total
    if (/^\s+$/.test(w)) return w
    const fill = p >= end ? 1 : p <= start ? 0 : (p - start) / (end - start)
    return <span key={i} className="lyr-w" style={{ ['--f' as string]: `${Math.round(fill * 100)}%` }}>{w}</span>
  })}</span>
}
