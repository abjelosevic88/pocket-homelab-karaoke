// Karaoke nights: the live one, the ones planned, the ones gone by.
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Book, Night } from '../lib/types'
import { useApp } from '../lib/app-context'
import { useAppState, announceChange, useOnChange } from '../lib/data'
import { useCrumb } from '../components/Layout'
import { Empty, Field, Loading, Modal, PageHeader, Segmented, Switch, errMsg, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'
import { fmtDateTime } from '../lib/format'

export function nightWhen(n: Night): string {
  if (n.status === 'live') return 'On now'
  if (n.starts_at) return fmtDateTime(n.starts_at.length === 10 ? n.starts_at + 'T00:00' : n.starts_at)
  return n.status === 'ended' && n.started_at ? fmtDateTime(n.started_at) : 'No day yet'
}

export default function Nights() {
  const [list, setList] = useState<Night[] | null>(null)
  const [sp, setSp] = useSearchParams()
  const [making, setMaking] = useState(sp.get('new') === '1')
  const navigate = useNavigate()
  useCrumb(undefined)
  const load = useCallback(() => { api.get<{ nights: Night[] }>(`${V1}/nights`).then(r => setList(r.nights)).catch(() => setList([])) }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  useEffect(() => { if (sp.get('new') === '1') { setMaking(true); setSp({}, { replace: true }) } }, [sp, setSp])
  const groups: [string, Night[]][] = list ? [['On now', list.filter(n => n.status === 'live')], ['Planned', list.filter(n => n.status === 'planned')], ['Gone by', list.filter(n => n.status === 'ended')]] : []
  return (
    <div className="page">
      <PageHeader title="Karaoke nights" sub="The big screen plays the queue; guests join on their phones with a code."
        actions={<button type="button" className="btn primary" onClick={() => setMaking(true)}><Icon name="plus" size={15} />New night</button>} />
      {!list ? <Loading /> : list.length === 0 ? <Empty title="No nights yet" icon="mic">Plan one — or start one right now. Put a laptop or a TV on the big screen, and everyone picks their songs on their phone.
        <div className="row mt" style={{ justifyContent: 'center' }}><button type="button" className="btn primary" onClick={() => setMaking(true)}><Icon name="plus" size={15} />New night</button></div></Empty>
        : groups.filter(([, l]) => l.length).map(([h, l]) => (
          <section key={h} className="stack">
            <h3 className="sec-h">{h}</h3>
            <div className="night-grid">{l.map(n => (
              <button key={n.id} type="button" className={`night-card ${n.status}`} onClick={() => navigate(`/nights/${n.id}`)}>
                <div className="row"><span className="grow ellipsis night-title">{n.title}</span>{n.status === 'live' && <span className="live-pill">live</span>}</div>
                <div className="muted small">{nightWhen(n)}{n.place ? ` · ${n.place}` : ''}</div>
                <div className="row small night-nums">
                  <span><Icon name="mic" size={13} />{n.sung} sung</span>
                  {n.status !== 'ended' && <span><Icon name="list-ordered" size={13} />{n.waiting} waiting</span>}
                  <span><Icon name="smartphone" size={13} />{n.guests} {n.guests === 1 ? 'guest' : 'guests'}</span>
                </div>
              </button>
            ))}</div>
          </section>
        ))}
      {making && <NightForm onClose={() => setMaking(false)} onSaved={n => { setMaking(false); announceChange(); navigate(`/nights/${n.id}`) }} />}
    </div>
  )
}

export function NightForm({ night, onClose, onSaved }: { night?: Night; onClose: () => void; onSaved: (n: Night) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const cal = useAppState()('calendar', 'calendar:write')
  const [books, setBooks] = useState<Book[]>([])
  const now = new Date()
  const def = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}T20:00`
  const [f, setF] = useState({
    title: night?.title || 'Karaoke night', starts_at: night?.starts_at ?? def, place: night?.place || '', about: night?.about || '',
    rotation: night?.rotation || settings?.rotation || 'fair', approve: night?.approve ?? settings?.approve ?? false, guest_youtube: night?.guest_youtube ?? settings?.guest_youtube ?? true,
    per_guest: night?.per_guest ?? settings?.per_guest ?? 2, book_id: night?.book_id ?? null as number | null, scores: night?.scores ?? settings?.scores ?? true, autoplay: night?.autoplay ?? settings?.autoplay ?? 15,
    calendar: !night && !!settings?.calendar_on_night,
  })
  useEffect(() => { api.get<{ books: Book[] }>(`${V1}/books`).then(r => setBooks(r.books)).catch(() => {}) }, [])
  const save = async () => {
    try {
      const n = night ? await api.put<Night>(`${V1}/nights/${night.id}`, f) : await api.post<Night>(`${V1}/nights`, { ...f, calendar: f.calendar && cal.ok })
      onSaved(n)
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={night ? 'The night' : 'A new karaoke night'} onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>{night ? 'Save' : 'Make the night'}</button></>}>
      <div className="stack">
        <Field label="Name"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} autoFocus /></Field>
        <div className="two-col tight">
          <Field label="When"><input type="datetime-local" value={f.starts_at} onChange={e => setF({ ...f, starts_at: e.target.value })} /></Field>
          <Field label="Where"><input value={f.place} onChange={e => setF({ ...f, place: e.target.value })} placeholder="Living room" /></Field>
        </div>
        <Field label="A word for the guests" help="On their phones when they join."><textarea rows={2} value={f.about} onChange={e => setF({ ...f, about: e.target.value })} placeholder="Snacks in the kitchen!" /></Field>
        <Field label="The queue"><Segmented size="sm" value={f.rotation} onChange={v => setF({ ...f, rotation: v })} options={[{ value: 'fair', label: 'Everyone in turn' }, { value: 'order', label: 'In the order asked' }]} /></Field>
        <div className="two-col tight">
          <Field label="Songs each in the queue" help="0 = no limit."><Select ariaLabel="Songs each" value={String(f.per_guest)} onChange={v => setF({ ...f, per_guest: Number(v) })} options={[0, 1, 2, 3, 5].map(n => ({ value: String(n), label: n === 0 ? 'No limit' : String(n) }))} /></Field>
          <Field label="The next song starts" help="By itself after a countdown, or when you press play."><Select ariaLabel="Autoplay" value={String(f.autoplay)} onChange={v => setF({ ...f, autoplay: Number(v) })} options={[{ value: '0', label: 'When I press play' }, { value: '10', label: 'After 10 seconds' }, { value: '15', label: 'After 15 seconds' }, { value: '30', label: 'After 30 seconds' }]} /></Field>
        </div>
        {!!books.length && <Field label="Guests choose from"><Select ariaLabel="Song book" value={f.book_id ? String(f.book_id) : ''} onChange={v => setF({ ...f, book_id: v ? Number(v) : null })} placeholder="All my songs" clearLabel="All my songs" options={books.map(b => ({ value: String(b.id), label: b.name, hint: String(b.songs) }))} /></Field>}
        <label className="check-row"><Switch checked={f.guest_youtube} onChange={v => setF({ ...f, guest_youtube: v })} label="YouTube" /><span>Guests may find songs on YouTube too</span></label>
        <label className="check-row"><Switch checked={f.approve} onChange={v => setF({ ...f, approve: v })} label="Approve" /><span>I say yes to each song first</span></label>
        <label className="check-row"><Switch checked={f.scores} onChange={v => setF({ ...f, scores: v })} label="Scores" /><span>Scores for fun (with a microphone on the big screen)</span></label>
        {!night && <label className="check-row"><Switch checked={f.calendar && cal.ok} disabled={!cal.ok} onChange={v => setF({ ...f, calendar: v })} label="Calendar" /><span>Put it in Pocket Calendar{!cal.ok && <span className="muted small"> — connect Pocket Calendar first</span>}</span></label>}
      </div>
    </Modal>
  )
}
