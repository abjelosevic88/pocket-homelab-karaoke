// Adding songs: upload files, find them on YouTube, take them from Pocket
// Music or Pocket Files, paste a link, or start with just the words.
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import type { Song, YTVideo } from '../lib/types'
import { fmtLen } from '../lib/karaoke'
import { useAppState, openAction } from '../lib/data'
import { announceChange } from '../lib/data'
import { useApp } from '../lib/app-context'
import { CoreHint, Empty, Modal, Spinner, Tabs, useDebounce, useToast } from './ui'
import { Icon } from './icons'

type Tab = 'upload' | 'youtube' | 'music' | 'files' | 'link' | 'words'

export function AddSongs({ onClose, start = 'youtube', query = '' }: { onClose: () => void; start?: Tab; query?: string }) {
  const [tab, setTab] = useState<Tab>(start)
  return (
    <Modal title="Add songs" onClose={onClose} size="lg" className="add-songs">
      <Tabs value={tab} onChange={setTab} tabs={[
        { id: 'youtube', label: 'YouTube', icon: 'youtube' }, { id: 'upload', label: 'Upload', icon: 'upload' }, { id: 'music', label: 'Pocket Music', icon: 'music' },
        { id: 'files', label: 'Pocket Files', icon: 'archive' }, { id: 'link', label: 'A link', icon: 'link' }, { id: 'words', label: 'Just the words', icon: 'text' },
      ]} />
      <div className="add-body">
        {tab === 'youtube' && <FromYouTube query={query} />}
        {tab === 'upload' && <FromUpload onDone={onClose} />}
        {tab === 'music' && <FromMusic />}
        {tab === 'files' && <FromFiles />}
        {tab === 'link' && <FromLink onDone={onClose} />}
        {tab === 'words' && <FromWords onDone={onClose} />}
      </div>
    </Modal>
  )
}

function FromYouTube({ query }: { query: string }) {
  const [q, setQ] = useState(query)
  const dq = useDebounce(q.trim(), 450)
  const [list, setList] = useState<YTVideo[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [added, setAdded] = useState<Record<string, number>>({})
  const { toast } = useToast()
  const navigate = useNavigate()
  useEffect(() => {
    if (!dq) { setList(null); return }
    let alive = true
    setBusy(true); setErr('')
    api.get<{ videos: YTVideo[] }>(`${V1}/youtube/search?q=${encodeURIComponent(dq)}`).then(r => { if (alive) setList(r.videos) }).catch(e => { if (alive) setErr((e as Error).message) }).finally(() => { if (alive) setBusy(false) })
    return () => { alive = false }
  }, [dq])
  const add = async (v: YTVideo) => {
    try {
      const s = await api.post<Song>(`${V1}/youtube`, v)
      setAdded(a => ({ ...a, [v.id]: s.id }))
      announceChange()
      toast(`“${s.title}” is in your songs`, 'ok', { label: 'Open', run: () => navigate(`/songs/${s.id}`) })
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack">
      <div className="search-box"><Icon name="search" size={16} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="A song or an artist — or paste a YouTube link" />{busy && <Spinner />}</div>
      <p className="muted small">Searches add “karaoke” for you (Settings → Songs). A karaoke video shows the words itself; a normal one plays with the words looked up for it. Videos play in YouTube’s player — keep a copy on this server for key change and voice cut.</p>
      {err && <div className="callout warn"><Icon name="alert" size={15} /><div>{err}</div></div>}
      {list && !list.length && !busy && <Empty title="Nothing found" icon="youtube">Try other words, or paste the video’s link.</Empty>}
      <div className="yt-list">
        {list?.map(v => {
          const have = added[v.id] || v.have
          return (
            <div key={v.id} className="yt-row">
              <img src={v.thumb} alt="" loading="lazy" />
              <div className="grow min0">
                <div className="yt-title">{v.title}</div>
                <div className="muted small">{v.channel}{v.duration_ms ? ` · ${fmtLen(v.duration_ms)}` : ''}{v.karaoke && <span className="badge ok sm">karaoke</span>}</div>
              </div>
              {have ? <button type="button" className="btn sm" onClick={() => navigate(`/songs/${have}`)}><Icon name="check" size={14} />In your songs</button>
                : <button type="button" className="btn sm primary" onClick={() => add(v)}><Icon name="plus" size={14} />Add</button>}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function FromUpload({ onDone }: { onDone: () => void }) {
  const input = useRef<HTMLInputElement>(null)
  const [files, setFiles] = useState<File[]>([])
  const [progress, setProgress] = useState<number | null>(null)
  const [done, setDone] = useState<Song[]>([])
  const [drag, setDrag] = useState(false)
  const { toast } = useToast()
  const navigate = useNavigate()
  // a song and the files beside it (same name: .cdg, .lrc) go up together
  const groups = (): File[][] => {
    const by: Record<string, File[]> = {}
    for (const f of files) {
      const stem = f.name.replace(/\.[^.]+$/, '').toLowerCase()
      ;(by[stem] = by[stem] || []).push(f)
    }
    return Object.values(by).filter(g => g.some(f => /\.(mp3|m4a|aac|ogg|oga|opus|flac|wav|weba|mp4|m4v|webm|mov|mkv|zip)$/i.test(f.name)))
  }
  const send = (g: File[]) => new Promise<Song>((resolve, reject) => {
    const fd = new FormData()
    for (const f of g) fd.append('file', f, f.name)
    const x = new XMLHttpRequest()
    x.open('POST', `${V1}/songs/upload`)
    x.upload.onprogress = e => { if (e.lengthComputable) setProgress(Math.round(e.loaded / e.total * 100)) }
    x.onload = () => {
      let j: { error?: string } & Song = {} as Song
      try { j = JSON.parse(x.responseText) } catch { /* not json */ }
      if (x.status >= 200 && x.status < 300) resolve(j as Song); else reject(new ApiError(x.status, j.error || x.statusText, j as unknown as Record<string, unknown>))
    }
    x.onerror = () => reject(new Error('The upload broke off'))
    x.send(fd)
  })
  const go = async () => {
    const gs = groups()
    if (!gs.length) { toast('Pick a song file (MP3, M4A, OGG, FLAC, WAV, MP4, WebM) or a .zip', 'err'); return }
    const out: Song[] = []
    for (const g of gs) {
      setProgress(0)
      try { out.push(await send(g)); setDone([...out]) } catch (e) { toast(`${g[0].name}: ${(e as Error).message}`, 'err') }
    }
    setProgress(null)
    announceChange()
    if (out.length === 1) { onDone(); navigate(`/songs/${out[0].id}`) }
    else if (out.length) toast(`${out.length} songs added`)
  }
  return (
    <div className="stack">
      <div className={`drop ${drag ? 'over' : ''}`} onDragOver={e => { e.preventDefault(); setDrag(true) }} onDragLeave={() => setDrag(false)}
        onDrop={e => { e.preventDefault(); setDrag(false); setFiles(f => [...f, ...Array.from(e.dataTransfer.files)]) }} onClick={() => input.current?.click()} role="button" tabIndex={0}>
        <Icon name="upload" size={26} />
        <b>Drop songs here, or choose them</b>
        <span className="muted small">MP3, M4A, OGG, FLAC, WAV, MP4, WebM — with a <b>.cdg</b> of the same name for karaoke discs (MP3+G), and an <b>.lrc</b> for timed words. A .zip of those works too.</span>
        <input ref={input} type="file" multiple hidden accept="audio/*,video/*,.cdg,.lrc,.zip,.mkv" onChange={e => setFiles(f => [...f, ...Array.from(e.target.files || [])])} />
      </div>
      {!!files.length && <ul className="file-list">{files.map((f, i) => <li key={i}><Icon name={/\.(mp4|webm|mov|mkv|m4v)$/i.test(f.name) ? 'video' : /\.cdg$/i.test(f.name) ? 'disc' : /\.lrc$/i.test(f.name) ? 'text' : 'music'} size={14} />{f.name}<span className="muted small">{(f.size / 1048576).toFixed(1)} MB</span>
        <button type="button" className="btn ghost icon-only sm" aria-label="Remove" onClick={() => setFiles(files.filter((_, j) => j !== i))}><Icon name="x" size={13} /></button></li>)}</ul>}
      {progress !== null && <div className="bar"><span style={{ width: `${progress}%` }} /></div>}
      {!!done.length && <p className="muted small">{done.length} done: {done.map(s => s.title).join(', ')}</p>}
      <div className="row"><button type="button" className="btn primary" onClick={go} disabled={!files.length || progress !== null}><Icon name="upload" size={15} />Add {groups().length > 1 ? `${groups().length} songs` : 'the song'}</button></div>
    </div>
  )
}

interface MusicSong { id: string; title: string; artist: string; album: string; duration_ms: number; cover: string; have?: number }

function FromMusic() {
  const appState = useAppState()
  const { app, ok, loading } = appState('music', 'music:play')
  const { settings } = useApp()
  const [q, setQ] = useState('')
  const dq = useDebounce(q.trim(), 350)
  const [list, setList] = useState<MusicSong[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [picked, setPicked] = useState<string[]>([])
  const { toast } = useToast()
  useEffect(() => {
    if (!dq || !ok) { setList(null); return }
    let alive = true
    setBusy(true)
    api.get<{ songs: MusicSong[] }>(`${V1}/apps/music?q=${encodeURIComponent(dq)}`).then(r => { if (alive) setList(r.songs) }).catch(e => toast((e as Error).message, 'err')).finally(() => { if (alive) setBusy(false) })
    return () => { alive = false }
  }, [dq, ok]) // eslint-disable-line react-hooks/exhaustive-deps
  if (loading) return <Spinner />
  if (!ok) return <AppMissing name="Pocket Music" detail={app?.fix || app?.detail} core={!!settings?.core_configured} />
  const add = async () => {
    try {
      const r = await api.post<{ songs: Song[]; text: string }>(`${V1}/apps/music`, { ids: picked })
      toast(r.text, 'ok', openAction(app?.open_url))
      setList(l => l?.map(s => picked.includes(s.id) ? { ...s, have: r.songs.find(x => x.source_ref === s.id)?.id } : s) || null)
      setPicked([])
      announceChange()
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack">
      <div className="search-box"><Icon name="search" size={16} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Search your music library" />{busy && <Spinner />}</div>
      <p className="muted small">Songs play from Pocket Music (nothing is copied) with the words it has for them. They have the voice in them: “less voice” turns it down.</p>
      <div className="yt-list">
        {list?.map(s => (
          <label key={s.id} className="yt-row pick">
            <input type="checkbox" checked={picked.includes(s.id)} disabled={!!s.have} onChange={e => setPicked(p => e.target.checked ? [...p, s.id] : p.filter(x => x !== s.id))} />
            <img src={s.cover} alt="" loading="lazy" className="sq" />
            <div className="grow min0"><div className="yt-title">{s.title}</div><div className="muted small">{s.artist}{s.album ? ` · ${s.album}` : ''}{s.duration_ms ? ` · ${fmtLen(s.duration_ms)}` : ''}</div></div>
            {s.have && <span className="badge ok">in your songs</span>}
          </label>
        ))}
        {list && !list.length && <Empty title="Nothing found" icon="music" />}
      </div>
      <div className="row"><button type="button" className="btn primary" disabled={!picked.length} onClick={add}><Icon name="plus" size={15} />Add {picked.length || ''} {picked.length === 1 ? 'song' : 'songs'}</button></div>
    </div>
  )
}

interface FileEntry { name: string; path: string; kind: string; size: number }

function FromFiles() {
  const appState = useAppState()
  const { app, ok, loading } = appState('files', 'files:read')
  const { settings } = useApp()
  const [path, setPath] = useState('/')
  const [list, setList] = useState<FileEntry[] | null>(null)
  const [picked, setPicked] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  useEffect(() => {
    if (!ok) return
    let alive = true
    setList(null)
    api.get<{ entries: FileEntry[] }>(`${V1}/apps/files?path=${encodeURIComponent(path)}`).then(r => { if (alive) setList(r.entries) }).catch(e => toast((e as Error).message, 'err'))
    return () => { alive = false }
  }, [path, ok]) // eslint-disable-line react-hooks/exhaustive-deps
  if (loading) return <Spinner />
  if (!ok) return <AppMissing name="Pocket Files" detail={app?.fix || app?.detail} core={!!settings?.core_configured} />
  const add = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ songs: Song[]; text: string; errors: string[] }>(`${V1}/apps/files/import`, { paths: picked })
      toast(r.text + (r.errors?.length ? ` (${r.errors.length} could not)` : ''))
      setPicked([])
      announceChange()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const up = path !== '/' ? path.replace(/\/[^/]+\/?$/, '') || '/' : ''
  return (
    <div className="stack">
      <div className="row"><Icon name="archive" size={15} /><b className="ellipsis">{path}</b></div>
      <div className="yt-list">
        {up && <button type="button" className="yt-row" onClick={() => { setPicked([]); setPath(up) }}><Icon name="arrow-up" size={16} /><span>Up</span></button>}
        {!list && <Spinner />}
        {list?.map(e => e.kind === 'folder'
          ? <button key={e.path} type="button" className="yt-row" onClick={() => setPath(e.path)}><Icon name="archive" size={16} /><span className="grow">{e.name}</span><Icon name="chevron-right" size={14} /></button>
          : <label key={e.path} className="yt-row pick">
              <input type="checkbox" disabled={e.kind === 'cdg' || e.kind === 'lrc'} checked={picked.includes(e.path)} onChange={x => setPicked(p => x.target.checked ? [...p, e.path] : p.filter(v => v !== e.path))} />
              <Icon name={e.kind === 'video' ? 'video' : e.kind === 'cdg' ? 'disc' : e.kind === 'lrc' ? 'text' : e.kind === 'zip' ? 'package' : 'music'} size={16} />
              <span className="grow ellipsis">{e.name}</span><span className="muted small">{(e.size / 1048576).toFixed(1)} MB</span>
            </label>)}
        {list && !list.length && <Empty title="No songs in this folder" icon="archive" />}
      </div>
      <p className="muted small">A .cdg or .lrc with the same name as a song comes along with it.</p>
      <div className="row"><button type="button" className="btn primary" disabled={!picked.length || busy} onClick={add}>{busy ? <Spinner /> : <Icon name="plus" size={15} />}Add {picked.length || ''} {picked.length === 1 ? 'song' : 'songs'}</button></div>
    </div>
  )
}

function FromLink({ onDone }: { onDone: () => void }) {
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const { toast } = useToast()
  const navigate = useNavigate()
  const go = async () => {
    setBusy(true)
    try {
      const s = await api.post<Song>(`${V1}/songs`, { url })
      announceChange(); onDone(); navigate(`/songs/${s.id}`)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack">
      <input autoFocus value={url} onChange={e => setUrl(e.target.value)} placeholder="https://www.youtube.com/watch?v=… or https://…/song.mp3" onKeyDown={e => { if (e.key === 'Enter' && url) go() }} />
      <p className="muted small">A YouTube link (watch, youtu.be, Shorts), or a link straight to a song file on the web.</p>
      <div className="row"><button type="button" className="btn primary" disabled={!url.trim() || busy} onClick={go}>{busy ? <Spinner /> : <Icon name="plus" size={15} />}Add</button></div>
    </div>
  )
}

function FromWords({ onDone }: { onDone: () => void }) {
  const [title, setTitle] = useState('')
  const [artist, setArtist] = useState('')
  const [lyrics, setLyrics] = useState('')
  const { toast } = useToast()
  const navigate = useNavigate()
  const go = async () => {
    try {
      const s = await api.post<Song>(`${V1}/songs`, { title, artist, lyrics })
      announceChange(); onDone(); navigate(`/songs/${s.id}`)
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack">
      <div className="two-col tight"><input value={title} onChange={e => setTitle(e.target.value)} placeholder="Title" autoFocus /><input value={artist} onChange={e => setArtist(e.target.value)} placeholder="Artist" /></div>
      <textarea rows={8} value={lyrics} onChange={e => setLyrics(e.target.value)} placeholder="Paste the words (or leave empty and look them up after)" />
      <p className="muted small">Good for a song you play on a guitar, or to get the words ready before the song file. Add the song file, a YouTube video or timing later on the song’s page.</p>
      <div className="row"><button type="button" className="btn primary" disabled={!title.trim()} onClick={go}><Icon name="plus" size={15} />Add the song</button></div>
    </div>
  )
}

export function AppMissing({ name, detail, core }: { name: string; detail?: string; core: boolean }) {
  return (
    <div className="callout info"><Icon name="info" size={15} /><div>
      <b>{name} is not connected.</b> {detail ? `${detail}.` : ''} Connect it in Settings → Connected apps.{!core && <CoreHint />}
    </div></div>
  )
}
