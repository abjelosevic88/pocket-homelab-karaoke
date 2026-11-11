// Sing a song on this device: the stage full screen, the key, less voice,
// the microphone through the speakers with echo, a score for fun and a
// recording. What was sung is kept in the history.
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Performance, Singer, Song } from '../lib/types'
import { KEY_LABEL, fmtLen, screenSettingsFrom, stageSong } from '../lib/karaoke'
import { wakeLock } from '../lib/audio'
import { useApp } from '../lib/app-context'
import { useAppState } from '../lib/data'
import { Stage, type StageHandle, type StageResult } from '../components/Stage'
import { Loading, Switch, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { AskButton } from '../components/AskButton'

export default function Sing() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { settings, user } = useApp()
  const { toast } = useToast()
  const [song, setSong] = useState<Song | null>(null)
  const [singers, setSingers] = useState<Singer[]>([])
  const [singer, setSinger] = useState('')
  const [practice, setPractice] = useState(true)
  const [key, setKey] = useState(0)
  const [voice, setVoice] = useState(0)
  const [volume, setVolume] = useState(90)
  const [mic, setMic] = useState(false)
  const [monitor, setMonitor] = useState(false)
  const [echo, setEcho] = useState(25)
  const [record, setRecord] = useState(false)
  const [playing, setPlaying] = useState(false)
  const [pos, setPos] = useState(0)
  const [result, setResult] = useState<(StageResult & { perf?: Performance }) | null>(null)
  const [translation, setTranslation] = useState<string[] | undefined>()
  const [panel, setPanel] = useState(false)
  const stage = useRef<StageHandle>(null)
  const filesState = useAppState()('files', 'files:write')
  const trState = useAppState()('translate', 'translate:run')

  useEffect(() => {
    api.get<{ song: Song }>(`${V1}/songs/${id}`).then(r => { setSong(r.song); setKey(r.song.key_shift); setVoice(r.song.kind === 'original' ? r.song.voice_cut : 0); setResult(null) }).catch(() => navigate('/songs'))
    api.get<{ singers: Singer[] }>(`${V1}/singers`).then(r => setSingers(r.singers)).catch(() => {})
  }, [id, navigate])
  useEffect(() => { if (settings) setEcho(settings.echo_amount) }, [settings])
  useEffect(() => {
    if (!song || !settings?.translate_to || !trState.ok || !song.lyrics || (song.language && song.language === settings.translate_to)) { setTranslation(undefined); return }
    api.get<{ lines: string[] }>(`${V1}/songs/${song.id}/translation?to=${settings.translate_to}`).then(r => setTranslation(r.lines)).catch(() => setTranslation(undefined))
  }, [song?.id, settings?.translate_to, trState.ok]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!playing || !settings?.keep_awake) return
    let release = () => {}
    wakeLock().then(r => { release = r })
    return () => release()
  }, [playing, settings?.keep_awake])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea, .sel')) return
      if (e.code === 'Space') { e.preventDefault(); toggle() }
      if (e.key === 'Escape') navigate(-1)
      if (e.key === "ArrowUp" && stage.current?.can.key) setKey(k => Math.min(6, k + 1))
      if (e.key === "ArrowDown" && stage.current?.can.key) setKey(k => Math.max(-6, k - 1))
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  const live = useMemo(() => song ? stageSong(song) : null, [song])
  if (!song || !live || !settings) return <div className="sing-page"><Loading /></div>
  const can = stage.current?.can || { key: !!live.media, voice: !!live.media, record: !!live.media }
  const st = screenSettingsFrom(settings)

  async function toggle() {
    if (!stage.current) return
    if (stage.current.playing()) { stage.current.pause(); return }
    setResult(null)
    await stage.current.play()
  }
  const setMicOn = async (on: boolean, mon = monitor, e = echo) => {
    const err = await stage.current?.mic(on, mon, e)
    if (err) { toast(err, 'err'); setMic(false); return }
    setMic(on)
  }
  const ended = async (r: StageResult) => {
    setPlaying(false)
    let perf: Performance | undefined
    if (r.sung_ms > 15000) {
      const sid = singers.find(g => String(g.id) === singer)
      try {
        perf = await api.post<Performance>(`${V1}/performances`, { song_id: song.id, singer: sid?.name || user?.name || '', singer_id: sid?.id || null, practice, score: settings.scores ? r.score : null, on_time: r.on_time, steady: r.steady, key_shift: key, sung_ms: r.sung_ms })
        if (r.recording && perf) {
          await fetch(`${V1}/performances/${perf.id}/recording`, { method: 'PUT', body: r.recording, headers: { 'Content-Type': r.recording.type.split(';')[0] || 'audio/webm' }, credentials: 'same-origin' })
          perf.has_recording = true
        }
      } catch (e) { toast(errMsg(e), 'err') }
    }
    setResult({ ...r, perf })
  }
  const toFiles = async () => {
    if (!result?.perf) return
    try { const r = await api.post<{ text: string }>(`${V1}/performances/${result.perf.id}/files`); toast(r.text) } catch (e) { toast(errMsg(e), 'err') }
  }
  const dur = song.duration_ms || 0
  return (
    <div className="sing-page">
      <header className="sing-top">
        <button type="button" className="btn ghost icon-only on-dark" aria-label="Back" title="Back (esc)" onClick={() => navigate(-1)}><Icon name="chevron-left" size={20} /></button>
        <div className="grow min0"><div className="ellipsis sing-name">{song.title}</div><div className="ellipsis small op7">{song.artist}</div></div>
        <AskButton />
        <button type="button" className="btn ghost icon-only on-dark" aria-label="Sound and microphone" title="Sound and microphone" onClick={() => setPanel(p => !p)}><Icon name="sliders" size={18} /></button>
      </header>
      <div className="sing-stage">
        <Stage ref={stage} song={live} settings={st} keyShift={key} voiceCut={voice} volume={volume} translation={translation}
          singer={singers.find(g => String(g.id) === singer)?.name}
          onStarted={() => { setPlaying(true); if (record && can.record) stage.current?.record(true) }}
          onTime={(ms, p) => { setPos(ms); setPlaying(p) }} onEnded={ended} onError={m => toast(m, 'err')} />
        {result && <div className="sing-result">
          <div className="res-card">
            {result.score !== null ? <><div className="res-score">{result.score}</div><div className="op7">points · on time {result.on_time}% · steady {result.steady}%</div></>
              : <div className="res-score sm">{result.sung_ms > 15000 ? 'Well sung!' : 'Stopped'}</div>}
            <p className="op7 small">{result.score === null && result.sung_ms > 15000 ? (mic ? 'The microphone did not hear enough singing for a score.' : 'Switch the microphone on for a score next time.') : 'For fun: it listens for singing on time and held notes — it does not know the melody.'}</p>
            <div className="row wrap center">
              <button type="button" className="btn primary" onClick={() => { setResult(null); stage.current?.seek(0); stage.current?.play() }}><Icon name="rotate-ccw" size={15} />Sing again</button>
              {result.perf?.has_recording && <a className="btn" href={`${V1}/performances/${result.perf.id}/recording`} target="_blank" rel="noreferrer"><Icon name="headphones" size={15} />Listen back</a>}
              {result.perf?.has_recording && <button type="button" className="btn" disabled={!filesState.ok} title={filesState.ok ? undefined : 'Connect Pocket Files'} onClick={toFiles}><Icon name="archive" size={15} />Keep in Pocket Files</button>}
              <Link className="btn ghost on-dark" to={`/songs/${song.id}`}>The song</Link>
            </div>
          </div>
        </div>}
      </div>
      <div className="sing-bar">
        <button type="button" className="btn primary round lg" onClick={toggle} aria-label={playing ? 'Pause' : 'Play'} title="Play / pause (space)"><Icon name={playing ? 'pause' : 'play'} size={22} /></button>
        <button type="button" className="btn ghost icon-only on-dark" aria-label="From the start" onClick={() => { stage.current?.seek(0) }}><Icon name="rotate-ccw" size={17} /></button>
        <span className="tnum small op7">{fmtLen(pos) || '0:00'}</span>
        <input type="range" className="grow seek" min={0} max={dur || 1} value={Math.min(pos, dur || 1)} onChange={e => stage.current?.seek(Number(e.target.value))} aria-label="Where in the song" disabled={!dur} />
        <span className="tnum small op7">{fmtLen(dur)}</span>
        <button type="button" className={`btn ghost icon-only on-dark ${mic ? 'lit' : ''}`} aria-label={mic ? 'Microphone off' : 'Microphone on'} title={mic ? 'The microphone is listening' : 'Microphone: a score and the pitch line'} onClick={() => setMicOn(!mic)}><Icon name={mic ? 'mic' : 'mic-off'} size={18} /></button>
      </div>
      {panel && <div className="sing-panel">
        <div className="sp-row"><span>Who sings</span><Select ariaLabel="Who sings" value={singer} onChange={setSinger} placeholder="Me" clearLabel="Me" options={singers.map(g => ({ value: String(g.id), label: `${g.emoji ? g.emoji + ' ' : ''}${g.name}` }))} /></div>
        <label className="sp-row"><span>Practice</span><Switch checked={practice} onChange={setPractice} label="Practice" /></label>
        <div className="sp-row"><span>Key <b>{KEY_LABEL(key)}</b></span>
          <div className="row"><button type="button" className="btn sm" disabled={!can.key || key <= -6} onClick={() => setKey(k => k - 1)}>−</button><button type="button" className="btn sm" disabled={!can.key || key === 0} onClick={() => setKey(0)}>0</button><button type="button" className="btn sm" disabled={!can.key || key >= 6} onClick={() => setKey(k => k + 1)}>+</button></div></div>
        <div className="sp-row"><span>Less voice <b>{voice}%</b></span><input type="range" min={0} max={100} step={5} value={voice} onChange={e => setVoice(Number(e.target.value))} disabled={!can.voice} /></div>
        <div className="sp-row"><span>Volume</span><input type="range" min={0} max={100} value={volume} onChange={e => setVolume(Number(e.target.value))} /></div>
        {!can.key && <p className="small op7">{song.source === 'youtube' ? 'A YouTube video plays in YouTube’s player: keep a copy on the song’s page to change the key, take the voice out and record.' : 'This song cannot be changed here.'}</p>}
        <hr />
        <label className="sp-row"><span>Microphone</span><Switch checked={mic} onChange={v => setMicOn(v)} label="Microphone" /></label>
        <label className="sp-row"><span>My voice through the speakers</span><Switch checked={monitor} disabled={!mic} onChange={v => { setMonitor(v); setMicOn(true, v, echo) }} label="Through the speakers" /></label>
        <div className="sp-row"><span>Echo</span><input type="range" min={0} max={100} value={echo} disabled={!mic || !monitor} onChange={e => { setEcho(Number(e.target.value)); stage.current?.mic(true, monitor, Number(e.target.value)) }} /></div>
        {monitor && <p className="small op7">Through the speakers your voice comes a moment late; headphones or a mixer sound best.</p>}
        <label className="sp-row"><span>Record me</span><Switch checked={record} disabled={!can.record || playing} onChange={setRecord} label="Record" /></label>
      </div>}
    </div>
  )
}
