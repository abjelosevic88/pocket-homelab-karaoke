// Song books: lists of songs for a night ("duets", "kids", "party starters").
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Book } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Confirm, Empty, Field, Loading, MenuButton, Modal, PageHeader, errMsg, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { SongPicker } from './Songs'

export default function Books() {
  const [list, setList] = useState<Book[] | null>(null)
  const [edit, setEdit] = useState<Partial<Book> | null>(null)
  const [add, setAdd] = useState<Book | null>(null)
  const [del, setDel] = useState<Book | null>(null)
  const navigate = useNavigate()
  const { toast } = useToast()
  useCrumb(undefined)
  const load = useCallback(() => { api.get<{ books: Book[] }>(`${V1}/books`).then(r => setList(r.books)).catch(() => setList([])) }, [])
  useEffect(() => { load() }, [load])
  const save = async () => {
    if (!edit) return
    try { if (edit.id) await api.put(`${V1}/books/${edit.id}`, edit); else await api.post(`${V1}/books`, edit); setEdit(null); load() } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <div className="page">
      <PageHeader title="Song books" sub="Lists of songs. Pick one for a night and guests choose only from it." actions={<button type="button" className="btn primary" onClick={() => setEdit({ name: '', about: '' })}><Icon name="plus" size={15} />New song book</button>} />
      {!list ? <Loading /> : list.length === 0 ? <Empty title="No song books yet" icon="book-open">Make one for duets, for the kids, or for a theme night.</Empty>
        : <div className="night-grid">{list.map(b => (
          <div key={b.id} className="night-card" role="link" tabIndex={0} onClick={() => navigate(`/songs?book=${b.id}`)} onKeyDown={e => { if (e.key === 'Enter') navigate(`/songs?book=${b.id}`) }}>
            <div className="row"><Icon name="book-open" size={16} /><span className="grow ellipsis night-title">{b.name}</span>
              <span onClick={e => e.stopPropagation()}><MenuButton label="More" items={[{ label: 'Add songs', icon: 'plus', onSelect: () => setAdd(b) }, { label: 'Rename', icon: 'edit', onSelect: () => setEdit(b) }, { label: 'Remove', icon: 'trash', danger: true, onSelect: () => setDel(b) }]}><Icon name="more" size={16} /></MenuButton></span></div>
            <div className="muted small">{b.songs} {b.songs === 1 ? 'song' : 'songs'}{b.about ? ` · ${b.about}` : ''}</div>
          </div>
        ))}</div>}
      {edit && <Modal title={edit.id ? 'The song book' : 'A new song book'} size="sm" onClose={() => setEdit(null)} footer={<><button type="button" className="btn" onClick={() => setEdit(null)}>Cancel</button><button type="button" className="btn primary" disabled={!edit.name?.trim()} onClick={save}>Save</button></>}>
        <div className="stack"><Field label="Name"><input value={edit.name || ''} onChange={e => setEdit({ ...edit, name: e.target.value })} autoFocus placeholder="Duets" /></Field>
          <Field label="About"><input value={edit.about || ''} onChange={e => setEdit({ ...edit, about: e.target.value })} placeholder="Two voices, one microphone each" /></Field></div>
      </Modal>}
      {add && <Modal title={`Add to ${add.name}`} onClose={() => { setAdd(null); load() }}>
        <SongPicker placeholder="Find a song to add" onPick={async s => { try { await api.put(`${V1}/books/${add.id}/songs/${s.id}`); toast(`${s.title} is in ${add.name}`) } catch (e) { toast(errMsg(e), 'err') } }} />
      </Modal>}
      {del && <Confirm title={`Remove ${del.name}?`} message="The songs stay in your library." confirmLabel="Remove" onCancel={() => setDel(null)} onConfirm={async () => { try { await api.del(`${V1}/books/${del.id}`); setDel(null); load() } catch (e) { toast(errMsg(e), 'err') } }} />}
    </div>
  )
}
