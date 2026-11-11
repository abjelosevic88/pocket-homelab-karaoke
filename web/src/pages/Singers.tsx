// Singers: the people who sing often, with what they sing and their scores.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Performance, Singer } from '../lib/types'
import { useAppState } from '../lib/data'
import { useCrumb } from '../components/Layout'
import { Confirm, Empty, Field, Loading, Modal, PageHeader, Spinner, errMsg, useDebounce, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { LineChart } from '../components/Chart'
import { PerfRow } from './SongPage'
import { ago } from '../lib/format'

const COLOURS = ['#7b2cbf', '#d6336c', '#1971c2', '#2f9e44', '#e8590c', '#0c8599', '#c2255c', '#5f3dc4', '#e67700', '#087f5b']
const EMOJI = ['🌟', '🎸', '🦋', '🚀', '🌈', '🎤', '🦊', '🐱', '🐼', '🌻', '🎧', '👑', '🔥', '🍀', '🎹', '🥁']

export default function Singers() {
  const [list, setList] = useState<Singer[] | null>(null)
  const [edit, setEdit] = useState<Partial<Singer> | null>(null)
  const navigate = useNavigate()
  useCrumb(undefined)
  const load = useCallback(() => { api.get<{ singers: Singer[] }>(`${V1}/singers`).then(r => setList(r.singers)).catch(() => setList([])) }, [])
  useEffect(() => { load() }, [load])
  return (
    <div className="page">
      <PageHeader title="Singers" sub="The people who sing with you often. Guests at a night need no card." actions={<button type="button" className="btn primary" onClick={() => setEdit({})}><Icon name="plus" size={15} />Add a singer</button>} />
      {!list ? <Loading /> : list.length === 0 ? <Empty title="No singers yet" icon="users">Add the household and friends: what they sing and their scores are kept for them.</Empty>
        : <div className="singer-grid">{list.map(g => (
          <button key={g.id} type="button" className="singer-card" onClick={() => navigate(`/singers/${g.id}`)}>
            <span className="singer-ava" style={{ background: g.colour }}>{g.emoji || g.name.slice(0, 1)}</span>
            <b className="ellipsis">{g.name}</b>
            <span className="muted small">{g.sung} sung{g.best !== null ? ` · best ${g.best}` : ''}</span>
            {g.last_at && <span className="muted small">last {ago(g.last_at)}</span>}
          </button>
        ))}</div>}
      {edit && <SingerForm singer={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load() }} />}
    </div>
  )
}

export function SingerForm({ singer, onClose, onSaved }: { singer: Partial<Singer>; onClose: () => void; onSaved: (g: Singer) => void }) {
  const [f, setF] = useState<Partial<Singer>>({ name: '', emoji: '', colour: '', ...singer })
  const [pick, setPick] = useState(false)
  const { toast } = useToast()
  const contacts = useAppState()('contacts', 'contacts:read')
  const save = async () => {
    try { const g = singer.id ? await api.put<Singer>(`${V1}/singers/${singer.id}`, f) : await api.post<Singer>(`${V1}/singers`, f); onSaved(g) } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={singer.id ? singer.name : 'A new singer'} onClose={onClose} size="sm" footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!f.name?.trim()} onClick={save}>Save</button></>}>
      <div className="stack">
        <Field label="Name"><div className="row"><input className="grow" value={f.name || ''} onChange={e => setF({ ...f, name: e.target.value })} autoFocus />
          <button type="button" className="btn" disabled={!contacts.ok} title={contacts.ok ? 'From Pocket Contacts' : 'Connect Pocket Contacts first'} onClick={() => setPick(true)}><Icon name="users" size={15} /></button></div></Field>
        <Field label="Emoji"><div className="chips">{EMOJI.map(e => <button key={e} type="button" className={`chip ${f.emoji === e ? 'on' : ''}`} onClick={() => setF({ ...f, emoji: f.emoji === e ? '' : e })}>{e}</button>)}</div></Field>
        <Field label="Colour"><div className="swatches">{COLOURS.map(c => <button key={c} type="button" className={`swatch ${f.colour === c ? 'on' : ''}`} style={{ background: c }} aria-label={c} onClick={() => setF({ ...f, colour: c })} />)}</div></Field>
        <Field label="Notes"><input value={f.notes || ''} onChange={e => setF({ ...f, notes: e.target.value })} placeholder="Loves ABBA; sings low" /></Field>
      </div>
      {pick && <ContactPick onClose={() => setPick(false)} onPick={c => { setF({ ...f, name: c.nickname || c.name.split(' ')[0] || c.name, contact_id: c.id }); setPick(false) }} />}
    </Modal>
  )
}

function ContactPick({ onClose, onPick }: { onClose: () => void; onPick: (c: { id: string; name: string; nickname: string }) => void }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<{ id: string; name: string; nickname: string }[] | null>(null)
  const { toast } = useToast()
  useEffect(() => { api.get<{ contacts: { id: string; name: string; nickname: string }[] }>(`${V1}/apps/contacts?q=${encodeURIComponent(dq)}`).then(r => setList(r.contacts)).catch(e => { toast(errMsg(e), 'err'); setList([]) }) }, [dq]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Modal title="From Pocket Contacts" onClose={onClose} size="sm">
      <div className="stack">
        <div className="search-box"><Icon name="search" size={16} /><input value={q} onChange={e => setQ(e.target.value)} autoFocus placeholder="A name" /></div>
        <div className="pick-list short">{!list ? <Spinner /> : list.map(c => <button key={c.id} type="button" className="pick-row" onClick={() => onPick(c)}><Icon name="user" size={15} /><span className="grow">{c.name}</span></button>)}</div>
      </div>
    </Modal>
  )
}

export function SingerPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [d, setD] = useState<{ singer: Singer; performances: Performance[] } | null>(null)
  const [edit, setEdit] = useState(false)
  const [del, setDel] = useState(false)
  const load = useCallback(() => { api.get<{ singer: Singer; performances: Performance[] }>(`${V1}/singers/${id}`).then(setD).catch(() => navigate('/singers')) }, [id, navigate])
  useEffect(() => { load() }, [load])
  useCrumb(d?.singer.name, '/singers')
  const points = useMemo(() => (d?.performances || []).filter(p => p.score !== null).slice(0, 40).reverse().map(p => ({ x: p.sung_at.slice(0, 10), y: p.score })), [d])
  const favs = useMemo(() => {
    const m: Record<string, { title: string; id: number | null; n: number }> = {}
    for (const p of d?.performances || []) { const k = p.title; m[k] = m[k] || { title: p.title, id: p.song_id, n: 0 }; m[k].n++ }
    return Object.values(m).sort((a, b) => b.n - a.n).slice(0, 6)
  }, [d])
  if (!d) return <Loading />
  const g = d.singer
  const scored = d.performances.filter(p => p.score !== null)
  const avg = scored.length ? Math.round(scored.reduce((s, p) => s + (p.score || 0), 0) / scored.length) : null
  return (
    <div className="page narrow">
      <div className="song-head">
        <span className="singer-ava lg" style={{ background: g.colour }}>{g.emoji || g.name.slice(0, 1)}</span>
        <div className="grow min0"><h1 className="song-h1">{g.name}</h1><div className="muted">{d.performances.length} sung{avg !== null ? ` · ${avg} on average` : ''}{g.notes ? ` · ${g.notes}` : ''}</div></div>
        <button type="button" className="btn" onClick={() => setEdit(true)}><Icon name="edit" size={15} />Change</button>
        <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={15} /></button>
      </div>
      {points.length > 2 && <section className="card pad-card"><h3 className="card-h">Scores</h3><LineChart points={points} height={160} zero /></section>}
      {!!favs.length && <section className="card pad-card"><h3 className="card-h">Sings most</h3><div className="chips">{favs.map(f => f.id ? <Link key={f.title} className="chip" to={`/songs/${f.id}`}>{f.title} · {f.n}</Link> : <span key={f.title} className="chip">{f.title} · {f.n}</span>)}</div></section>}
      <section className="card pad-card"><h3 className="card-h">Everything</h3>{d.performances.length === 0 ? <p className="muted small">Nothing yet.</p> : <div className="perf-list">{d.performances.map(p => <PerfRow key={p.id} p={p} showSong />)}</div>}</section>
      {edit && <SingerForm singer={g} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load() }} />}
      {del && <Confirm title={`Remove ${g.name}?`} message="What they sang stays in the history, with their name." confirmLabel="Remove" onCancel={() => setDel(false)} onConfirm={async () => { try { await api.del(`${V1}/singers/${g.id}`); navigate('/singers') } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
