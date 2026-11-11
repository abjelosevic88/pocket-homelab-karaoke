// Left column: add songs, the sections, the night on now or coming up, and
// a few favourites to sing.
import { useCallback, useEffect, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useOnChange } from '../lib/data'
import type { Home } from '../lib/types'
import { Icon } from './icons'

export function Sidebar({ onCreate, onNavigate }: { onCreate: () => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [home, setHome] = useState<Home | null>(null)
  const load = useCallback(() => { api.get<Home>(`${V1}/home`).then(setHome).catch(() => {}) }, [])
  useEffect(() => { load() }, [load, loc.pathname])
  useOnChange(load)

  const title = settings?.title || status?.title || 'Pocket Karaoke'
  const path = loc.pathname
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number | string; hint?: string; strong?: boolean }[] = [
    { to: '/', label: 'Home', icon: 'home', active: path === '/' },
    { to: '/songs', label: 'Songs', icon: 'music', active: path.startsWith('/songs'), count: home?.songs || undefined },
    { to: '/nights', label: 'Nights', icon: 'mic', active: path.startsWith('/nights'), count: home?.live ? 'live' : undefined, strong: !!home?.live, hint: home?.live ? 'A night is on now' : undefined },
    { to: '/singers', label: 'Singers', icon: 'users', active: path.startsWith('/singers') },
    { to: '/books', label: 'Song books', icon: 'book-open', active: path.startsWith('/books') },
    { to: '/history', label: 'History', icon: 'history', active: path.startsWith('/history') },
  ]
  const n = home?.live || home?.next
  return (
    <aside className="sidebar" aria-label="Pocket Karaoke">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Karaoke' ? title : <>Pocket <b>Karaoke</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <button type="button" className="btn primary create-btn" onClick={onCreate} title="Add songs (N)">
        <Icon name="plus" size={15} strokeWidth={2.2} />Add songs
      </button>

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(x => (
          <NavLink key={x.to} to={x.to} onClick={onNavigate} title={x.hint} className={() => `sb-nav-item ${x.active ? 'active' : ''}`}>
            <Icon name={x.icon} size={16} />{x.label}
            {!!x.count && <span className={`sb-count ${x.strong ? 'strong' : ''}`} title={x.hint}>{x.count}</span>}
          </NavLink>
        ))}
      </nav>

      {n && <section className="sb-sec">
        <h4 className="sb-h">{n.status === 'live' ? 'On now' : 'Coming up'}</h4>
        <button type="button" className={`sb-row sb-list ${path === `/nights/${n.id}` ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/nights/${n.id}`) }} title={n.title}>
          <Icon name={n.status === 'live' ? 'mic' : 'calendar'} size={15} className="sb-list-ico" />
          <span className="grow ellipsis">{n.title}</span>
          {n.status === 'live' ? <span className="sb-count strong">live</span> : <span className="muted small nowrap">{n.starts_at.slice(5, 10).split('-').reverse().join('.')}</span>}
        </button>
      </section>}

      {!!home?.favourites.length && <section className="sb-sec">
        <h4 className="sb-h"><Link to="/songs?filter=fav" onClick={onNavigate} className="sb-h-link">Favourites</Link></h4>
        {home.favourites.slice(0, 6).map(s => (
          <button key={s.id} type="button" className={`sb-row sb-list ${path === `/songs/${s.id}` ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/songs/${s.id}`) }} title={`${s.title} — ${s.artist}`}>
            <Icon name="star" size={15} className="sb-list-ico" />
            <span className="grow ellipsis">{s.title}</span>
          </button>
        ))}
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pkar:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
