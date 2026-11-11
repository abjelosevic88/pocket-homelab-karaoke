// Running a karaoke night: what is on the big screen and the buttons for it,
// the songs asked for, the queue (drag to change it), the guests, and — once
// it is over — how it went.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Guest, LiveEntry, LiveMsg, LiveState, Night, Performance, Singer, Song } from '../lib/types'
import { KEY_LABEL, fmtLen } from '../lib/karaoke'
import { useApp } from '../lib/app-context'
import { announceChange, openAction, useAppState } from '../lib/data'
import { useCrumb } from '../components/Layout'
import { Confirm, CopyField, Empty, Loading, MenuButton, Modal, Spinner, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { QRCode } from '../components/QRCode'
import { NightForm, nightWhen } from './Nights'
import { SongPicker } from './Songs'
import { PerfRow } from './SongPage'

interface Payload { night: Night; live: LiveState; guests: Guest[]; performances: Performance[]; join_url: string; screen_url: string }

export default function NightPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const [data, setData] = useState<Payload | null>(null)
  const [live, setLive] = useState<LiveState | null>(null)
  const [pos, setPos] = useState(0)
  const [gone, setGone] = useState(false)
  const [adding, setAdding] = useState(false)
  const [invite, setInvite] = useState(false)
  const [editing, setEditing] = useState(false)
  const [ending, setEnding] = useState(false)
  const [del, setDel] = useState(false)
  const [suggest, setSuggest] = useState(false)
  const [react, setReact] = useState<{ emoji: string; name: string; id: number } | null>(null)
  const chat = useAppState()('chat', 'chat:write')
  const music = useAppState()('music', 'playlists:write')
  const cal = useAppState()('calendar', 'calendar:write')

  const load = useCallback(() => api.get<Payload>(`${V1}/nights/${id}`).then(d => { setData(d); setLive(d.live) }).catch(() => setGone(true)), [id])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    let es: EventSource | null = null
    let alive = true
    const open = () => {
      es = new EventSource(`${V1}/nights/${id}/live/stream`)
      es.onmessage = e => {
        const m = JSON.parse(e.data) as LiveMsg
        if (m.type === 'state') setLive(m)
        else if (m.type === 'progress') setPos(m.position_ms)
        else if (m.type === 'react') setReact({ emoji: m.emoji, name: m.name, id: Date.now() })
        else if (m.type === 'gone') { es?.close(); setGone(true) }
      }
      es.onerror = () => { es?.close(); if (alive) setTimeout(open, 3000) }
    }
    open()
    return () => { alive = false; es?.close() }
  }, [id])
  // the guests and what was sung follow the live state
  const lastDone = live?.done?.length
  useEffect(() => { if (data) api.get<Payload>(`${V1}/nights/${id}`).then(d => setData(d)).catch(() => {}) }, [lastDone, live?.guests, live?.status]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { setPos(0) }, [live?.current?.id])
  useCrumb(data?.night.title, '/nights')
  if (gone) return <Empty title="This night is not here" icon="mic"><Link to="/nights">All nights</Link></Empty>
  if (!data || !live) return <Loading />
  const n = data.night

  const control = async (action: string, value = 0) => {
    try { const r = await api.post<{ night: Night; live: LiveState }>(`${V1}/nights/${n.id}/control`, { action, value }); setLive(r.live); setData(d => d ? { ...d, night: r.night } : d) } catch (e) { toast(errMsg(e), 'err') }
  }
  const sendChat = async (what: 'invite' | 'results') => {
    try { const r = await api.post<{ text: string; url: string }>(`${V1}/nights/${n.id}/chat`, { what }); toast(r.text, 'ok', openAction(r.url)) } catch (e) { toast(errMsg(e), 'err') }
  }
  const toCalendar = async () => {
    try { const r = await api.post<{ text: string; url: string }>(`${V1}/nights/${n.id}/calendar`); toast(r.text, 'ok', openAction(r.url)); load() } catch (e) { toast(errMsg(e), 'err') }
  }
  const playlist = async () => {
    try { const r = await api.post<{ text: string; url: string }>(`${V1}/nights/${n.id}/playlist`); toast(r.text, 'ok', openAction(r.url)) } catch (e) { toast(errMsg(e), 'err') }
  }
  const openScreen = () => window.open(`/screen/${n.screen_key}`, 'pkar-screen', 'noopener')
  const isLive = live.status === 'live'
  const cur = live.current
  const phase = live.phase
  const dur = live.song?.duration_ms || cur?.duration_ms || 0
  const canSound = !!live.song?.media

  return (
    <div className="page night-page">
      <div className="night-head">
        <div className="grow min0">
          <div className="row"><h1 className="song-h1 ellipsis">{n.title}</h1>{isLive && <span className="live-pill">live</span>}</div>
          <div className="muted">{nightWhen(n)}{n.place ? ` · ${n.place}` : ''} · code <b className="tnum">{n.join_code}</b></div>
        </div>
        <div className="row wrap">
          {n.status === 'planned' && <button type="button" className="btn primary" onClick={() => control('start')}><Icon name="play" size={15} />Start the night</button>}
          {n.status === 'ended' && <button type="button" className="btn" onClick={() => control('start')}><Icon name="rotate-ccw" size={15} />Start it again</button>}
          <button type="button" className="btn" onClick={openScreen} title="Open on the TV or the laptop by the TV"><Icon name="tv" size={15} />Big screen</button>
          <button type="button" className="btn" onClick={() => setInvite(true)}><Icon name="qr" size={15} />Invite</button>
          <MenuButton label="More" items={[
            { label: 'Change the night', icon: 'edit', onSelect: () => setEditing(true) },
            { label: 'Send the invitation to Pocket Chat', icon: 'message', disabled: !chat.ok, hint: chat.ok ? undefined : 'not connected', onSelect: () => sendChat('invite') },
            { label: n.calendar_ref ? 'In Pocket Calendar' : 'Put it in Pocket Calendar', icon: 'calendar', disabled: !cal.ok || !n.starts_at, onSelect: () => n.calendar_ref ? window.open(n.calendar_ref, '_blank', 'noopener') : toCalendar() },
            { label: 'Suggest songs (AI)', icon: 'sparkles', disabled: !settings?.ai_help, onSelect: () => setSuggest(true) },
            'sep',
            ...(isLive ? [{ label: 'End the night', icon: 'stop', onSelect: () => setEnding(true) }] : []),
            { label: 'Remove the night', icon: 'trash', danger: true, onSelect: () => setDel(true) },
          ]}><Icon name="more" size={18} /></MenuButton>
        </div>
      </div>

      {n.status === 'planned' && <div className="callout info"><Icon name="info" size={15} /><div>Guests can already join with the code <b>{n.join_code}</b> and ask for songs. Press <b>Start the night</b> when you begin, and open the <b>Big screen</b> on the TV.</div></div>}

      {isLive && <section className={`now-card ${phase}`}>
        <div className="now-top">
          <span className="muted small">{phase === 'idle' ? 'Nothing on the screen' : phase === 'next' ? 'Up next on the screen' : phase === 'score' ? 'Just sang' : phase === 'paused' ? 'Paused' : 'Singing now'}</span>
          <span className="spacer" />
          <span className="muted small now-counts"><span><Icon name="tv" size={13} />{live.screens} {live.screens === 1 ? 'screen' : 'screens'}</span><span><Icon name="smartphone" size={13} />{live.phones} on phones</span></span>
        </div>
        {cur ? <>
          <div className="now-song"><b>{cur.singer}</b> · {cur.title}<span className="muted"> — {cur.artist}</span></div>
          <div className="now-progress"><span className="tnum small muted">{fmtLen(pos) || '0:00'}</span><div className="bar grow"><span style={{ width: `${dur ? Math.min(100, pos / dur * 100) : 0}%` }} /></div><span className="tnum small muted">{fmtLen(dur)}</span></div>
          <div className="row wrap now-buttons">
            {phase === 'singing' ? <button type="button" className="btn primary" onClick={() => control('pause')}><Icon name="pause" size={15} />Pause</button>
              : (phase === 'next' || phase === 'paused') ? <button type="button" className="btn primary" onClick={() => control('play')}><Icon name="play" size={15} />{phase === 'next' ? 'Start the song' : 'Go on'}</button> : null}
            {(phase === 'singing' || phase === 'paused') && <button type="button" className="btn" onClick={() => control('restart')}><Icon name="rotate-ccw" size={15} />From the start</button>}
            {phase !== 'score' && <button type="button" className="btn" onClick={() => control('skip')}><Icon name="skip-forward" size={15} />Skip</button>}
            {phase === 'score' && <button type="button" className="btn primary" onClick={() => control('next')}><Icon name="skip-forward" size={15} />Next singer</button>}
            {(phase === 'next' || phase === 'singing' || phase === 'paused') && <button type="button" className="btn ghost" onClick={() => control('stop')} title="Back to the top of the queue">Not now</button>}
            {react && <span key={react.id} className="react-pop">{react.emoji} <small className="muted">{react.name}</small></span>}
            {live.cheers > 0 && <span className="muted small">👏 {live.cheers}</span>}
          </div>
          <div className="now-sound">
            <div className="ns"><span>Key <b>{KEY_LABEL(live.key_now)}</b></span><div className="row"><button type="button" className="btn sm" disabled={!canSound || live.key_now <= -6} onClick={() => control('key', live.key_now - 1)}>−</button><button type="button" className="btn sm" disabled={!canSound || live.key_now === 0} onClick={() => control('key', 0)}>0</button><button type="button" className="btn sm" disabled={!canSound || live.key_now >= 6} onClick={() => control('key', live.key_now + 1)}>+</button></div></div>
            <div className="ns"><span>Less voice <b>{live.voice_now}%</b></span><input type="range" min={0} max={100} step={10} value={live.voice_now} disabled={!canSound} onChange={e => control('voice', Number(e.target.value))} /></div>
            <div className="ns"><span>Volume <b>{live.volume}</b></span><input type="range" min={0} max={100} step={5} value={live.volume} onChange={e => control('volume', Number(e.target.value))} /></div>
          </div>
          {!canSound && live.song?.youtube && <p className="muted small">A YouTube video: key and voice change work once a copy is kept on this server (the song’s page).</p>}
        </> : <div className="row wrap">
          <span className="grow muted">{live.queue.length ? `${live.queue.length} waiting.` : 'The queue is empty: add a song, or let the guests pick on their phones.'}</span>
          {!!live.queue.length && <button type="button" className="btn primary" onClick={() => control('next')}><Icon name="play" size={15} />Call {live.queue[0].singer}</button>}
        </div>}
        {live.screens === 0 && <p className="small warn-text"><Icon name="alert" size={13} /> No big screen is open. <button type="button" className="linkish" onClick={openScreen}>Open it</button> on the TV (or this device).</p>}
      </section>}

      {!!live.asked?.length && <section className="card pad-card">
        <h3 className="card-h">Asked for <span className="badge accent">{live.asked.length}</span></h3>
        <div className="q-list">{live.asked.map(e => <AskedRow key={e.id} e={e} night={n.id} />)}</div>
      </section>}

      {n.status !== 'ended' && <section className="card pad-card">
        <div className="row"><h3 className="card-h grow">The queue {live.queue.length ? <span className="muted">· {live.queue.length}</span> : null}</h3>
          <button type="button" className="btn sm primary" onClick={() => setAdding(true)}><Icon name="plus" size={14} />Add a song</button></div>
        {live.queue.length === 0 ? <p className="muted small">Nobody is waiting. Guests ask for songs at <b>{data.join_url.replace(/^https?:\/\//, '')}</b>.</p>
          : <QueueList night={n} queue={live.queue} />}
        <p className="muted small">{n.rotation === 'fair' ? 'Everyone in turn: a new song goes after everyone else’s next one.' : 'In the order asked.'} Drag to change it.</p>
      </section>}

      <div className="two-col">
        <section className="card pad-card">
          <h3 className="card-h">Guests <span className="muted">· {data.guests.length}</span></h3>
          {data.guests.length === 0 ? <p className="muted small">Nobody has joined from a phone yet.</p> : <div className="guest-list">{data.guests.map(g => <GuestRow key={g.id} g={g} night={n.id} onChange={load} />)}</div>}
        </section>
        <section className="card pad-card">
          <div className="row"><h3 className="card-h grow">Sung tonight <span className="muted">· {data.performances.length}</span></h3>
            {!!data.performances.length && <MenuButton label="Results" className="btn sm" items={[
              { label: 'Send the results to Pocket Chat', icon: 'message', disabled: !chat.ok, onSelect: () => sendChat('results') },
              { label: 'Make a Pocket Music playlist', icon: 'music', disabled: !music.ok, onSelect: playlist },
            ]}><Icon name="share" size={14} />Share</MenuButton>}
          </div>
          {data.performances.length === 0 ? <p className="muted small">Nothing yet.</p> : <>
            <Podium perfs={data.performances} />
            <div className="perf-list">{data.performances.map(p => <PerfRow key={p.id} p={p} showSong />)}</div>
          </>}
        </section>
      </div>

      {adding && <AddEntry night={n} guests={data.guests} onClose={() => setAdding(false)} />}
      {invite && <Modal title="Invite to the night" onClose={() => setInvite(false)} footer={<><button type="button" className="btn" disabled={!chat.ok} onClick={() => sendChat('invite')}><Icon name="message" size={15} />Send to Pocket Chat</button><button type="button" className="btn primary" onClick={() => setInvite(false)}>Done</button></>}>
        <div className="stack center">
          <QRCode value={data.join_url} size={220} />
          <div className="big-code">{n.join_code}</div>
          <CopyField value={data.join_url} />
          <p className="muted small">Guests scan this or open the link and type the code — no account needed. The big screen shows it too.</p>
          <div className="row"><span className="muted small">Big screen link (keep it to yourself):</span></div>
          <CopyField value={data.screen_url} />
        </div>
      </Modal>}
      {editing && <NightForm night={n} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); load(); announceChange() }} />}
      {ending && <Confirm title="End the night?" danger={false} confirmLabel="End it" message="The queue stops, guests can no longer ask for songs, and what was sung stays in the history." onCancel={() => setEnding(false)} onConfirm={() => { setEnding(false); control('end') }} />}
      {del && <Confirm title="Remove this night?" confirmLabel="Remove" message="Its queue and guests go. What was sung stays in the history." onCancel={() => setDel(false)} onConfirm={async () => { try { await api.del(`${V1}/nights/${n.id}`); announceChange(); navigate('/nights') } catch (e) { toast(errMsg(e), 'err') } }} />}
      {suggest && <Suggest night={n} onClose={() => setSuggest(false)} />}
    </div>
  )
}

function Podium({ perfs }: { perfs: Performance[] }) {
  const top = perfs.filter(p => p.score !== null).sort((a, b) => (b.score || 0) - (a.score || 0)).slice(0, 3)
  const cheered = [...perfs].sort((a, b) => b.cheers - a.cheers)[0]
  if (!top.length && !cheered?.cheers) return null
  return (
    <div className="podium">
      {top.map((p, i) => <div key={p.id} className={`pod p${i + 1}`}><span className="pod-medal">{['🥇', '🥈', '🥉'][i]}</span><b className="ellipsis">{p.singer}</b><span className="muted small ellipsis">{p.title}</span><span className="pod-score">{p.score}</span></div>)}
      {cheered?.cheers > 0 && <div className="pod cheer"><span className="pod-medal">👏</span><b className="ellipsis">{cheered.singer}</b><span className="muted small">most cheered · {cheered.cheers}</span></div>}
    </div>
  )
}

function AskedRow({ e, night }: { e: LiveEntry; night: number }) {
  const { toast } = useToast()
  const act = async (status: 'waiting' | 'skipped') => { try { await api.put(`${V1}/nights/${night}/queue/${e.id}`, { status }) } catch (x) { toast(errMsg(x), 'err') } }
  return (
    <div className="q-row asked">
      <div className="grow min0"><div className="ellipsis"><b>{e.singer}</b> · <Link to={`/songs/${e.song_id}`}>{e.title}</Link></div><div className="muted small ellipsis">{e.artist}{e.source === 'youtube' ? ' · YouTube' : ''}</div></div>
      <button type="button" className="btn sm primary" onClick={() => act('waiting')}><Icon name="check" size={14} />Yes</button>
      <button type="button" className="btn sm ghost" onClick={() => act('skipped')}>No</button>
    </div>
  )
}

function QueueList({ night, queue }: { night: Night; queue: LiveEntry[] }) {
  const [order, setOrder] = useState(queue)
  const drag = useRef<number | null>(null)
  const { toast } = useToast()
  useEffect(() => { setOrder(queue) }, [queue])
  const save = async (list: LiveEntry[]) => { try { await api.put(`${V1}/nights/${night.id}/queue`, { ids: list.map(e => e.id) }) } catch (e) { toast(errMsg(e), 'err') } }
  const move = (from: number, to: number) => {
    if (to < 0 || to >= order.length || from === to) return
    const l = [...order]
    const [x] = l.splice(from, 1)
    l.splice(to, 0, x)
    setOrder(l)
    save(l)
  }
  const remove = async (e: LiveEntry) => { try { await api.del(`${V1}/nights/${night.id}/queue/${e.id}`) } catch (x) { toast(errMsg(x), 'err') } }
  const key = async (e: LiveEntry, k: number) => { try { await api.put(`${V1}/nights/${night.id}/queue/${e.id}`, { key_shift: k }) } catch (x) { toast(errMsg(x), 'err') } }
  return (
    <ol className="q-list">
      {order.map((e, i) => (
        <li key={e.id} className="q-row" draggable onDragStart={() => { drag.current = i }} onDragOver={ev => ev.preventDefault()} onDrop={() => { if (drag.current !== null) move(drag.current, i); drag.current = null }}>
          <span className="q-grip" aria-hidden><Icon name="grip" size={15} /></span>
          <span className="q-n tnum">{i + 1}</span>
          <div className="grow min0">
            <div className="ellipsis"><b>{e.singer}</b> · <Link to={`/songs/${e.song_id}`}>{e.title}</Link></div>
            <div className="muted small ellipsis">{e.artist}{e.duration_ms ? ` · ${fmtLen(e.duration_ms)}` : ''}{e.key_shift ? ` · key ${KEY_LABEL(e.key_shift)}` : ''}{e.note ? ` · ${e.note}` : ''}</div>
          </div>
          <MenuButton label="Change" items={[
            { label: 'To the top', icon: 'arrow-up', disabled: i === 0, onSelect: () => move(i, 0) },
            { label: 'Up one', icon: 'chevron-up', disabled: i === 0, onSelect: () => move(i, i - 1) },
            { label: 'Down one', icon: 'chevron-down', disabled: i === order.length - 1, onSelect: () => move(i, i + 1) },
            'sep',
            { label: 'Key up', icon: 'plus', disabled: e.key_shift >= 6, onSelect: () => key(e, e.key_shift + 1) },
            { label: 'Key down', icon: 'minus', disabled: e.key_shift <= -6, onSelect: () => key(e, e.key_shift - 1) },
            'sep',
            { label: 'Take out of the queue', icon: 'trash', danger: true, onSelect: () => remove(e) },
          ]}><Icon name="more" size={16} /></MenuButton>
        </li>
      ))}
    </ol>
  )
}

function GuestRow({ g, night, onChange }: { g: Guest; night: number; onChange: () => void }) {
  const { toast } = useToast()
  const block = async () => { try { await api.put(`${V1}/nights/${night}/guests/${g.id}`, { blocked: !g.blocked }); onChange() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <div className="guest-row">
      <span className="g-avatar">{g.name.slice(0, 1).toUpperCase()}</span>
      <div className="grow min0"><div className="ellipsis">{g.name}{g.blocked && <span className="badge warn sm">paused</span>}</div><div className="muted small">{g.sung} sung · {g.waiting} waiting</div></div>
      <button type="button" className="btn sm ghost" onClick={block} title={g.blocked ? 'Let them ask for songs again' : 'Stop their requests for now'}>{g.blocked ? 'Let ask again' : 'Pause'}</button>
    </div>
  )
}

function AddEntry({ night, guests, onClose }: { night: Night; guests: Guest[]; onClose: () => void }) {
  const [song, setSong] = useState<Song | null>(null)
  const [singers, setSingers] = useState<Singer[]>([])
  const [who, setWho] = useState('')
  const [name, setName] = useState('')
  const [front, setFront] = useState(false)
  const { toast } = useToast()
  useEffect(() => { api.get<{ singers: Singer[] }>(`${V1}/singers`).then(r => setSingers(r.singers)).catch(() => {}) }, [])
  const add = async () => {
    if (!song) return
    const body: Record<string, unknown> = { song_id: song.id, front }
    if (who.startsWith('s')) body.singer_id = Number(who.slice(1))
    else if (who.startsWith('g')) body.guest_id = Number(who.slice(1))
    if (name.trim()) body.singer = name.trim()
    try { await api.post(`${V1}/nights/${night.id}/queue`, body); toast(`${song.title} is in the queue`); onClose() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={song ? song.title : 'Add a song to the queue'} onClose={onClose} footer={song ? <><button type="button" className="btn" onClick={() => setSong(null)}>Another song</button><button type="button" className="btn primary" onClick={add}><Icon name="plus" size={15} />Add</button></> : undefined}>
      {!song ? <SongPicker onPick={setSong} /> : <div className="stack">
        <p className="muted">{song.artist}{song.duration_ms ? ` · ${fmtLen(song.duration_ms)}` : ''}</p>
        <Select ariaLabel="Who sings" value={who} onChange={setWho} placeholder="Who sings?" options={[
          ...singers.map(g => ({ value: `s${g.id}`, label: `${g.emoji ? g.emoji + ' ' : ''}${g.name}`, group: 'Singers' })),
          ...guests.map(g => ({ value: `g${g.id}`, label: g.name, group: 'Guests tonight' })),
        ]} />
        <input value={name} onChange={e => setName(e.target.value)} placeholder="…or a name (“Ana & Marko” for a duet)" />
        <label className="check-row"><input type="checkbox" checked={front} onChange={e => setFront(e.target.checked)} /><span>Straight to the top of the queue</span></label>
      </div>}
    </Modal>
  )
}

function Suggest({ night, onClose }: { night: Night; onClose: () => void }) {
  const [ask, setAsk] = useState('')
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState<{ picks: { song: Song; why: string }[]; note: string } | null>(null)
  const { toast } = useToast()
  const run = async () => {
    setBusy(true)
    try { setRes(await api.post(`${V1}/ai/suggest`, { ask, night_id: night.id })) } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const queue = async (s: Song) => { try { await api.post(`${V1}/nights/${night.id}/queue`, { song_id: s.id, singer: 'Everyone' }); toast(`${s.title} is in the queue`) } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <Modal title="Suggest songs" onClose={onClose}>
      <div className="stack">
        <div className="row"><input className="grow" value={ask} onChange={e => setAsk(e.target.value)} placeholder="Who is here, the mood… (“kids and grandparents”, “80s, loud”)" onKeyDown={e => { if (e.key === 'Enter') run() }} />
          <button type="button" className="btn primary" disabled={busy} onClick={run}>{busy ? <Spinner /> : <Icon name="sparkles" size={15} />}Suggest</button></div>
        <p className="muted small">Pocket AI picks from your own songs only.</p>
        {res?.note && <p className="small">{res.note}</p>}
        <div className="pick-list">{res?.picks.map(p => <div key={p.song.id} className="pick-row"><span className="grow min0"><span className="block-line ellipsis"><b>{p.song.title}</b> · {p.song.artist}</span><span className="muted small block-line">{p.why}</span></span><button type="button" className="btn sm" onClick={() => queue(p.song)}>Queue</button></div>)}</div>
      </div>
    </Modal>
  )
}
