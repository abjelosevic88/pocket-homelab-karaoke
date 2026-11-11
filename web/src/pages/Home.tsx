// The start page: a night on now (or the next one), favourites to sing,
// what was sung last, and songs that still need words.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Home as HomeT } from '../lib/types'
import { useOnChange } from '../lib/data'
import { useCrumb } from '../components/Layout'
import { AddSongs } from '../components/AddSongs'
import { Empty, Loading } from '../components/ui'
import { Icon } from '../components/icons'
import { SongCover, SongRow } from './Songs'
import { PerfRow } from './SongPage'
import { nightWhen } from './Nights'

export default function Home() {
  const [h, setH] = useState<HomeT | null>(null)
  const [adding, setAdding] = useState(false)
  const navigate = useNavigate()
  useCrumb(undefined)
  const load = useCallback(() => { api.get<HomeT>(`${V1}/home`).then(setH).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  if (!h) return <Loading />
  if (h.songs === 0 && h.nights === 0) return (
    <div className="page">
      <Empty title="Karaoke at home" icon="mic">
        Add songs — karaoke videos from YouTube, your own MP3s and MP4s, karaoke discs (MP3 + CDG), or songs from Pocket Music — then sing them here, or start a night: the TV plays the queue and everyone picks songs on their phone.
        <div className="row mt" style={{ justifyContent: 'center' }}><button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add songs</button><Link className="btn" to="/nights?new=1"><Icon name="mic" size={15} />Plan a night</Link></div>
      </Empty>
      {adding && <AddSongs onClose={() => { setAdding(false); load() }} />}
    </div>
  )
  const n = h.live || h.next
  return (
    <div className="page">
      {n ? <button type="button" className={`hero-night ${n.status}`} onClick={() => navigate(`/nights/${n.id}`)}>
        <div className="hero-mic">🎤</div>
        <div className="grow min0">
          <div className="small op8">{n.status === 'live' ? 'On now' : 'Next night'}</div>
          <div className="hero-title ellipsis">{n.title}</div>
          <div className="small op8">{nightWhen(n)}{n.place ? ` · ${n.place}` : ''} · code {n.join_code}{n.status === 'live' ? ` · ${n.waiting} waiting · ${n.guests} on phones` : ''}</div>
        </div>
        <span className="btn on-hero">{n.status === 'live' ? 'Run it' : 'Open'}<Icon name="chevron-right" size={15} /></span>
      </button> : <div className="hero-night empty">
        <div className="hero-mic">🎤</div>
        <div className="grow"><div className="hero-title">A karaoke night</div><div className="small op8">The TV plays the queue; everyone picks songs on their phone with a code.</div></div>
        <Link className="btn on-hero" to="/nights?new=1"><Icon name="plus" size={15} />Plan one</Link>
      </div>}

      <div className="stats-row">
        <Link to="/songs" className="stat"><span className="stat-l">Songs</span><span className="stat-v">{h.songs}</span></Link>
        <Link to="/nights" className="stat"><span className="stat-l">Nights</span><span className="stat-v">{h.nights}</span></Link>
        <Link to="/singers" className="stat"><span className="stat-l">Singers</span><span className="stat-v">{h.singers}</span></Link>
        <Link to="/songs?filter=nowords" className="stat"><span className="stat-l">Need words</span><span className={`stat-v ${h.no_words_total ? 'warn-text' : ''}`}>{h.no_words_total}</span></Link>
      </div>

      {!!h.copying.length && <div className="callout info"><Icon name="download" size={15} /><div>Keeping copies of {h.copying.length} YouTube {h.copying.length === 1 ? 'song' : 'songs'}: {h.copying.map(s => `${s.title}${s.copy_state === 'working' ? ` (${s.copy_progress}%)` : ''}`).join(', ')}.</div></div>}

      {!!h.favourites.length && <section className="stack">
        <div className="row"><h3 className="sec-h grow">Favourites</h3><Link to="/songs?filter=fav" className="small">All</Link></div>
        <div className="fav-grid">{h.favourites.map(s => (
          <Link key={s.id} to={`/sing/${s.id}`} className="fav-card" title={`Sing ${s.title}`}>
            <SongCover s={s} size={120} />
            <span className="fav-play"><Icon name="mic" size={16} /></span>
            <b className="ellipsis">{s.title}</b><span className="muted small ellipsis">{s.artist}</span>
          </Link>
        ))}</div>
      </section>}

      <div className="two-col">
        <section className="card pad-card">
          <div className="row"><h3 className="card-h grow">Sung last</h3><Link to="/history" className="small">History</Link></div>
          {h.performances.length === 0 ? <p className="muted small">Nothing yet. Pick a song and press <b>Sing</b>.</p> : <div className="perf-list">{h.performances.map(p => <PerfRow key={p.id} p={p} showSong />)}</div>}
        </section>
        <section className="card pad-card">
          <div className="row"><h3 className="card-h grow">Added last</h3><button type="button" className="btn sm" onClick={() => setAdding(true)}><Icon name="plus" size={14} />Add songs</button></div>
          <div className="song-list compact">{h.recent.map(s => <SongRow key={s.id} s={s} onChange={load} />)}</div>
          {!!h.no_words.length && <>
            <h4 className="sub-h">Still without words</h4>
            <div className="song-list compact">{h.no_words.map(s => <SongRow key={s.id} s={s} onChange={load} />)}</div>
          </>}
        </section>
      </div>
      {adding && <AddSongs onClose={() => { setAdding(false); load() }} />}
    </div>
  )
}
