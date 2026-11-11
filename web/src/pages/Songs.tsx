// The song library: search, filters, song books, and adding songs.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import type { Book, Song } from '../lib/types'
import { SOURCE, coverOf, fmtLen, langName, wordsOf } from '../lib/karaoke'
import { announceChange, useOnChange } from '../lib/data'
import { useCrumb } from '../components/Layout'
import { AddSongs } from '../components/AddSongs'
import { Empty, Loading, PageHeader, Segmented, useDebounce, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { Icon } from '../components/icons'

export default function Songs() {
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 250)
  const source = sp.get('source') || ''
  const lang = sp.get('language') || ''
  const book = sp.get('book') || ''
  const filter = sp.get('filter') || ''
  const sort = sp.get('sort') || 'title'
  const [data, setData] = useState<{ songs: Song[]; total: number; languages: string[] } | null>(null)
  const [books, setBooks] = useState<Book[]>([])
  const [adding, setAdding] = useState(sp.get('add') === '1')
  useCrumb(undefined)
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n, { replace: true }) }
  const load = useCallback(() => {
    const p = new URLSearchParams({ q: dq, source, language: lang, book, sort: book && sort === 'title' ? 'book' : sort, limit: '500' })
    if (filter === 'fav') p.set('favourite', '1')
    if (filter === 'nowords') p.set('no_words', '1')
    api.get<{ songs: Song[]; total: number; languages: string[] }>(`${V1}/songs?${p}`).then(setData).catch(() => setData({ songs: [], total: 0, languages: [] }))
  }, [dq, source, lang, book, sort, filter])
  useEffect(() => { load() }, [load])
  useEffect(() => { api.get<{ books: Book[] }>(`${V1}/books`).then(r => setBooks(r.books)).catch(() => {}) }, [])
  useOnChange(load)
  useEffect(() => { if (sp.get('add') === '1') { setAdding(true); set('add', '') } }, [sp]) // eslint-disable-line react-hooks/exhaustive-deps
  const bookName = books.find(b => String(b.id) === book)?.name
  return (
    <div className="page">
      <PageHeader title={bookName ? `Song book: ${bookName}` : 'Songs'} sub={data ? `${data.total.toLocaleString()} ${data.total === 1 ? 'song' : 'songs'}${bookName ? ' in this book' : ''}` : undefined}
        actions={<button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add songs</button>} />
      <div className="toolbar row wrap">
        <div className="search-box grow" style={{ minWidth: 200 }}><Icon name="search" size={16} /><input value={q} onChange={e => { setQ(e.target.value); set('q', e.target.value) }} placeholder="Title, artist or a tag" /></div>
        <Select ariaLabel="Where from" value={source} onChange={v => set('source', v)} placeholder="Anywhere" clearLabel="Anywhere" options={Object.entries(SOURCE).map(([k, v]) => ({ value: k, label: v.label }))} />
        {!!data?.languages.length && <Select ariaLabel="Language" value={lang} onChange={v => set('language', v)} placeholder="Any language" clearLabel="Any language" options={data.languages.map(l => ({ value: l, label: langName(l) }))} />}
        {!!books.length && <Select ariaLabel="Song book" value={book} onChange={v => set('book', v)} placeholder="All songs" clearLabel="All songs" options={books.map(b => ({ value: String(b.id), label: b.name, hint: String(b.songs) }))} />}
        <Select ariaLabel="Order" value={sort} onChange={v => set('sort', v)} options={[{ value: 'title', label: 'By title' }, { value: 'artist', label: 'By artist' }, { value: 'added', label: 'Added last' }, { value: 'sung', label: 'Sung last' }, { value: 'plays', label: 'Sung most' }]} />
      </div>
      <Segmented size="sm" value={filter} onChange={v => set('filter', v)} options={[{ value: '', label: 'All' }, { value: 'fav', label: 'Favourites' }, { value: 'nowords', label: 'No words yet' }]} />
      {!data ? <Loading /> : data.songs.length === 0
        ? <Empty title={q || source || lang || filter ? 'No songs like that' : 'No songs yet'} icon="music">
            {!q && !source && <>Find karaoke videos on YouTube, upload MP3s, MP4s and karaoke discs (MP3 + CDG), or bring songs in from Pocket Music and Pocket Files.<div className="row mt" style={{ justifyContent: 'center' }}><button type="button" className="btn primary" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add songs</button></div></>}
          </Empty>
        : <div className="song-list">{data.songs.map(s => <SongRow key={s.id} s={s} onChange={load} />)}</div>}
      {adding && <AddSongs onClose={() => { setAdding(false); load() }} query={q} />}
    </div>
  )
}

export function SongCover({ s, size = 44 }: { s: Song; size?: number }) {
  const [bad, setBad] = useState(false)
  const src = coverOf(s)
  return (
    <span className="song-cover" style={{ width: size, height: size }}>
      {src && !bad ? <img src={src} alt="" loading="lazy" onError={() => setBad(true)} /> : <Icon name={s.is_video ? 'video' : 'music'} size={Math.round(size * 0.42)} />}
    </span>
  )
}

export function SongRow({ s, onChange, extra }: { s: Song; onChange?: () => void; extra?: React.ReactNode }) {
  const navigate = useNavigate()
  const { toast } = useToast()
  const words = wordsOf(s)
  const fav = async (e: React.MouseEvent) => {
    e.stopPropagation()
    try { await api.put(`${V1}/songs/${s.id}`, { favourite: !s.favourite }); onChange?.(); announceChange() } catch (err) { toast((err as Error).message, 'err') }
  }
  return (
    <div className="song-row" role="link" tabIndex={0} onClick={() => navigate(`/songs/${s.id}`)} onKeyDown={e => { if (e.key === 'Enter') navigate(`/songs/${s.id}`) }}>
      <SongCover s={s} />
      <div className="grow min0">
        <div className="song-title ellipsis">{s.title}</div>
        <div className="muted small ellipsis">{s.artist || 'Unknown artist'}{s.duration_ms ? ` · ${fmtLen(s.duration_ms)}` : ''}</div>
      </div>
      <div className="song-meta hide-sm">
        <span className="src-tag" title={SOURCE[s.source]?.label}><Icon name={SOURCE[s.source]?.icon || 'music'} size={13} />{SOURCE[s.source]?.label}</span>
        <span className={`words-tag ${words.ok ? '' : 'missing'}`}>{words.text}</span>
        {s.copy_state === 'working' && <span className="badge">copying {s.copy_progress}%</span>}
      </div>
      {s.best !== undefined && <span className="best" title="Best score">{s.best}</span>}
      {extra}
      <button type="button" className={`btn ghost icon-only fav ${s.favourite ? 'on' : ''}`} aria-label={s.favourite ? 'Not a favourite' : 'Favourite'} title={s.favourite ? 'A favourite' : 'Make it a favourite'} onClick={fav}><Icon name="star" size={16} /></button>
      <Link className="btn sm sing-btn" to={`/sing/${s.id}`} onClick={e => e.stopPropagation()} title="Sing it now on this device"><Icon name="mic" size={14} /><span className="hide-sm">Sing</span></Link>
    </div>
  )
}

/** Picks a song from the library (for the queue, a song book). */
export function SongPicker({ onPick, book, placeholder = 'Find a song' }: { onPick: (s: Song) => void; book?: number | null; placeholder?: string }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 200)
  const [list, setList] = useState<Song[] | null>(null)
  useEffect(() => {
    const p = new URLSearchParams({ q: dq, limit: '40', sort: dq ? 'title' : 'plays' })
    if (book) p.set('book', String(book))
    api.get<{ songs: Song[] }>(`${V1}/songs?${p}`).then(r => setList(r.songs)).catch(() => setList([]))
  }, [dq, book])
  return (
    <div className="stack">
      <div className="search-box"><Icon name="search" size={16} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder={placeholder} /></div>
      <div className="pick-list">
        {list?.map(s => (
          <button key={s.id} type="button" className="pick-row" onClick={() => onPick(s)}>
            <SongCover s={s} size={34} />
            <span className="grow min0"><span className="ellipsis block-line">{s.title}</span><span className="muted small ellipsis block-line">{s.artist}{s.duration_ms ? ` · ${fmtLen(s.duration_ms)}` : ''}</span></span>
            {!wordsOf(s).ok && <span className="words-tag missing">no words</span>}
          </button>
        ))}
        {list && !list.length && <p className="muted small pad">No songs found.</p>}
      </div>
    </div>
  )
}
