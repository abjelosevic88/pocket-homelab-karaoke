// What was sung: the numbers (most sung songs and singers, a year of
// singing) and every performance, with recordings.
import { useEffect, useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import type { Performance, Stat } from '../lib/types'
import { useApp } from '../lib/app-context'
import { useCrumb } from '../components/Layout'
import { Empty, Loading, PageHeader, Segmented } from '../components/ui'
import { HBars, Heatmap } from '../components/Chart'
import { PerfRow } from './SongPage'

interface Stats { songs: Stat[]; singers: Stat[]; per_day: Record<string, number>; total: number; scored: number; average: number; best: Performance | null; minutes: number }

export default function History() {
  const [period, setPeriod] = useState<'month' | 'year' | 'all'>('year')
  const [st, setSt] = useState<Stats | null>(null)
  const [perfs, setPerfs] = useState<Performance[] | null>(null)
  const [only, setOnly] = useState<'' | 'nights' | 'practice' | 'rec'>('')
  const { settings } = useApp()
  useCrumb(undefined)
  useEffect(() => { api.get<Stats>(`${V1}/stats?period=${period}`).then(setSt).catch(() => {}) }, [period])
  useEffect(() => { api.get<{ performances: Performance[] }>(`${V1}/performances?limit=500`).then(r => setPerfs(r.performances)).catch(() => setPerfs([])) }, [])
  const days = useMemo(() => {
    if (!st) return []
    const out: { date: string; minutes: number }[] = []
    const d = new Date()
    for (let i = 364; i >= 0; i--) {
      const x = new Date(d.getFullYear(), d.getMonth(), d.getDate() - i)
      const k = `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, '0')}-${String(x.getDate()).padStart(2, '0')}`
      out.push({ date: k, minutes: st.per_day[k] || 0 })
    }
    return out
  }, [st])
  const shown = (perfs || []).filter(p => only === '' || (only === 'nights' && p.night_id) || (only === 'practice' && p.practice) || (only === 'rec' && p.has_recording))
  return (
    <div className="page">
      <PageHeader title="History" sub="Every song sung, at nights and alone." actions={<Segmented size="sm" value={period} onChange={setPeriod} options={[{ value: 'month', label: 'Month' }, { value: 'year', label: 'Year' }, { value: 'all', label: 'Ever' }]} />} />
      {!st ? <Loading /> : st.total === 0 ? <Empty title="Nothing sung yet" icon="mic">Sing a song, or run a night: everything sung shows here.</Empty> : <>
        <div className="stats-row">
          <div className="stat"><span className="stat-l">Songs sung</span><span className="stat-v">{st.total}</span></div>
          <div className="stat"><span className="stat-l">Minutes</span><span className="stat-v">{st.minutes}</span></div>
          <div className="stat"><span className="stat-l">Average score</span><span className="stat-v">{st.scored ? Math.round(st.average) : '—'}</span></div>
          <div className="stat"><span className="stat-l">Best</span><span className="stat-v">{st.best?.score ?? '—'}</span><span className="stat-s ellipsis">{st.best ? `${st.best.singer} · ${st.best.title}` : ''}</span></div>
        </div>
        <section className="card pad-card"><h3 className="card-h">A year of singing</h3><Heatmap days={days} weekStart={settings?.week_start || 1} fmt={m => `${m} ${m === 1 ? 'song' : 'songs'}`} /></section>
        <div className="two-col">
          <section className="card pad-card"><h3 className="card-h">Sung most</h3><HBars items={st.songs.map(s => ({ label: s.name, value: s.n, hint: s.extra }))} format={v => String(v)} /></section>
          <section className="card pad-card"><h3 className="card-h">Who sang most</h3><HBars items={st.singers.map(s => ({ label: s.name, value: s.n, hint: s.best !== null ? `best ${s.best}` : '' }))} format={v => String(v)} /></section>
        </div>
      </>}
      <section className="card pad-card">
        <div className="row wrap"><h3 className="card-h grow">Everything</h3><Segmented size="sm" value={only} onChange={setOnly} options={[{ value: '', label: 'All' }, { value: 'nights', label: 'At nights' }, { value: 'practice', label: 'Practice' }, { value: 'rec', label: 'Recorded' }]} /></div>
        {!perfs ? <Loading /> : shown.length === 0 ? <p className="muted small">Nothing here.</p> : <div className="perf-list">{shown.map(p => <PerfRow key={p.id} p={p} showSong />)}</div>}
      </section>
    </div>
  )
}
