import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { APP_INFO, STATE_WORDS } from '../lib/apps'
import { LANGS } from '../lib/karaoke'

type TabId = 'general' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'How the words show, the microphone and the score, new songs, new nights — and dates and the app title.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'What reaches your phone: a night tomorrow, a guest asking for a song, a YouTube copy done.' },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Music, Files, Photos, Calendar, Chat, Contacts, Translate, Habits and Alerts.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who helps you: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Each person has their own songs and nights.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and other tools.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Export everything, bring an export back, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="The words" icon="text" sub="On the Sing page and the big screen.">
        <SettingRow title="Size"><Segmented size="sm" value={f.lyrics_size} onChange={v => setF({ ...f, lyrics_size: v })} options={[{ value: 'm', label: 'Medium' }, { value: 'l', label: 'Large' }, { value: 'xl', label: 'Huge' }]} /></SettingRow>
        <SettingRow title="How they light up" help="Word by word as they are sung, or the whole line at once."><Segmented size="sm" value={f.lyrics_style} onChange={v => setF({ ...f, lyrics_style: v })} options={[{ value: 'sweep', label: 'Word by word' }, { value: 'line', label: 'The line' }]} /></SettingRow>
        <SettingRow title="Show the next line"><Switch checked={f.next_line} onChange={v => setF({ ...f, next_line: v })} label="Next line" /></SettingRow>
        <SettingRow title="Count in" help="Three dots before the first words."><Switch checked={f.count_in} onChange={v => setF({ ...f, count_in: v })} label="Count in" /></SettingRow>
        <SettingRow title="A translation under each line" help={f.translate_via ? 'Pocket Translate translates the words of a song in another language.' : 'Needs Pocket Translate (Connected apps).'}>
          <Select ariaLabel="Translate to" value={f.translate_to} onChange={v => setF({ ...f, translate_to: v })} placeholder="Off" clearLabel="Off" options={LANGS.filter(l => l.value).map(l => ({ ...l, disabled: !f.translate_via }))} />
        </SettingRow>
      </Section>
      <Section title="Microphone and score" icon="mic" sub="The microphone of the device that plays the song.">
        <SettingRow title="A score for fun" help="It listens for singing on time and held notes; it does not know the melody."><Switch checked={f.scores} onChange={v => setF({ ...f, scores: v })} label="Scores" /></SettingRow>
        <SettingRow title="Pitch line" help="The notes you sing, drawn along the bottom."><Switch checked={f.pitch_line} onChange={v => setF({ ...f, pitch_line: v })} label="Pitch line" /></SettingRow>
        <SettingRow title="Echo" help="For your voice through the speakers."><input type="range" min={0} max={100} value={f.echo_amount} onChange={e => setF({ ...f, echo_amount: Number(e.target.value) })} /></SettingRow>
        <SettingRow title="Keep the screen on" help="While a song plays."><Switch checked={f.keep_awake} onChange={v => setF({ ...f, keep_awake: v })} label="Keep awake" /></SettingRow>
      </Section>
      <Section title="New songs" icon="music">
        <SettingRow title="Look up the words" help="On LRCLIB (a free library of timed words) when a song comes in; only its title and artist are sent."><Switch checked={f.lyrics_lookup} onChange={v => setF({ ...f, lyrics_lookup: v })} label="Look up" /></SettingRow>
        <SettingRow title="YouTube searches add" help="So the results are karaoke videos with the words on screen."><input value={f.youtube_suffix} onChange={e => setF({ ...f, youtube_suffix: e.target.value })} placeholder="karaoke" /></SettingRow>
        <SettingRow title="Keep a copy of every YouTube song" help={f.copies_possible ? 'On this server (yt-dlp): plays without YouTube, with key change, less voice and recording.' : 'Not possible on this box (yt-dlp is not there, or DOWNLOADS=off).'}><Switch checked={f.keep_copies} disabled={!f.copies_possible} onChange={v => setF({ ...f, keep_copies: v })} label="Keep copies" /></SettingRow>
        <SettingRow title="Copies up to"><Segmented size="sm" value={f.video_quality} onChange={v => setF({ ...f, video_quality: v })} options={[{ value: 480, label: '480p' }, { value: 720, label: '720p' }, { value: 1080, label: '1080p' }]} /></SettingRow>
        <SettingRow title="Less voice for songs with the voice" help="Where a song from Pocket Music or an upload with the singer starts."><input type="range" min={0} max={100} step={5} value={f.default_voice} onChange={e => setF({ ...f, default_voice: Number(e.target.value) })} /></SettingRow>
      </Section>
      <Section title="New nights" icon="mic" sub="Each night can change these.">
        <SettingRow title="The queue"><Segmented size="sm" value={f.rotation} onChange={v => setF({ ...f, rotation: v })} options={[{ value: 'fair', label: 'Everyone in turn' }, { value: 'order', label: 'In the order asked' }]} /></SettingRow>
        <SettingRow title="Songs each in the queue"><Segmented size="sm" value={f.per_guest} onChange={v => setF({ ...f, per_guest: v })} options={[{ value: 1, label: '1' }, { value: 2, label: '2' }, { value: 3, label: '3' }, { value: 0, label: 'No limit' }]} /></SettingRow>
        <SettingRow title="The next song starts"><Segmented size="sm" value={f.autoplay} onChange={v => setF({ ...f, autoplay: v })} options={[{ value: 0, label: 'On a press' }, { value: 10, label: '10 s' }, { value: 15, label: '15 s' }, { value: 30, label: '30 s' }]} /></SettingRow>
        <SettingRow title="Guests may find songs on YouTube"><Switch checked={f.guest_youtube} onChange={v => setF({ ...f, guest_youtube: v })} label="Guest YouTube" /></SettingRow>
        <SettingRow title="I say yes to each song first"><Switch checked={f.approve} onChange={v => setF({ ...f, approve: v })} label="Approve" /></SettingRow>
      </Section>
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/karaoke" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to push" icon="bell">
        <SettingRow title="A night is tomorrow" help="With the code guests join with, and how many songs still have no words. In Pocket Alerts too, when it is connected."><Switch checked={f.notify_night} onChange={v => setF({ ...f, notify_night: v })} label="Night tomorrow" /></SettingRow>
        <SettingRow title="A guest asks for a song" help="Only on nights where you say yes to each song first."><Switch checked={f.notify_request} onChange={v => setF({ ...f, notify_request: v })} label="Requests" /></SettingRow>
        <SettingRow title="A YouTube copy is done (or failed)"><Switch checked={f.notify_copy} onChange={v => setF({ ...f, notify_copy: v })} label="Copies" /></SettingRow>
        <SettingRow title="Reminders go at" help="Server time."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test push</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'music', title: 'Pocket Music — sing your own library', sub: 'Sing songs from your music library (played from there, with their timed words); after a night, a playlist of what was sung.', token: 'Pocket Music → Settings → API tokens', port: '8200' },
  { slug: 'files', title: 'Pocket Files — karaoke files and recordings', sub: 'Bring songs in from your files (MP3, MP4, MP3 + CDG, .lrc beside them); keep recordings of performances there.', token: 'Pocket Files → Settings → API tokens', port: '8187' },
  { slug: 'photos', title: 'Pocket Photos — pictures between songs', sub: 'Show photos from one of your albums on the big screen while nobody sings.', token: 'Pocket Photos → Settings → API tokens', port: '8194' },
  { slug: 'calendar', title: 'Pocket Calendar — the night in your calendar', sub: 'Put a planned night in your calendar with the link guests join with.', token: 'Pocket Calendar → Settings → API tokens', port: '8190' },
  { slug: 'chat', title: 'Pocket Chat — invitations and results', sub: 'Send the invitation (the code and the link) and how the night went to a chat.', token: 'Pocket Chat → Settings → API tokens', port: '8247' },
  { slug: 'contacts', title: 'Pocket Contacts — singers from your address book', sub: 'Add the people you sing with as singers.', token: 'Pocket Contacts → Settings → API tokens', port: '8191' },
  { slug: 'translate', title: 'Pocket Translate — the words in your language', sub: 'Show each line of a song in your language under the words.', token: 'Pocket Translate → Settings → API tokens', port: '8251' },
  { slug: 'habits', title: 'Pocket Habits — a singing habit', sub: 'Tick your singing habit when you practise a song for more than a minute.', token: 'Pocket Habits → Settings → API tokens', port: '8168' },
  { slug: 'alerts', title: 'Pocket Alerts — your alerts inbox', sub: 'The day before a karaoke night.', token: 'Pocket Alerts → Settings → API tokens', port: '8217' },
]
function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [APPS.map(a => f?.[`${a.slug}_mode`]).join()])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. Karaoke reads or adds things in them when you press a button (and, if you switch it on below, when you plan a night, practise or a night ends), and everything here works without them. Through <b>Pocket Core</b> Karaoke acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        const st = states?.find(x => x.slug === a.slug)
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={st && st.state !== 'on' && st.state !== 'off' ? <span className={`badge ${st.state === 'partly' ? 'ok' : 'warn'}`}>{STATE_WORDS[st.state]}</span> : connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {st?.fix && st.state !== 'on' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.fix}.</p>}
          </Section>
        )
      })}
      <WhereThingsGo f={f} setF={setF} />
      <Section title="Other Pocket apps" icon="layers" sub="Karaoke offers your songs and nights to the rest of the box through Pocket Core: Pocket Day shows what was sung, Pocket Search finds songs by a line of their words, Pocket AI can tell you about your nights or put a song in tonight’s queue. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>karaoke:read</dt><dd className="muted">Your songs with their words, singers, song books, nights and who sang what with the scores.</dd></div>
          <div><dt>karaoke:write</dt><dd className="muted">Add songs, plan a night and put a song in a live night’s queue; that app may change only what it added, and can never start, end or delete your own nights.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>karaoke.song_added</code>, <code>night_planned</code>, <code>night_started</code>, <code>performed</code>, <code>night_ended</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="Only when you press its button: it tidies words pasted from a web page into singable lines (never writing words itself), and suggests songs from your own library for the room. You see what it did before anything is kept." >
        <SettingRow title="AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: Karaoke does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own songs, nights, settings and connections. Admins manage accounts; they never see other people's songs or nights.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account goes, with their songs, nights and history." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try {
      setUser(await api.put(`${V1}/auth/me`, f))
      setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name" help="The singer’s name when you sing alone."><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email" help="The same email as in Pocket Core, so signing in finds your account."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or send a recording from a Siri Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /summary</code></dt><dd className="muted">Songs, nights, what was sung this week, the night on now — for Homepage.</dd></div>
          <div><dt><code>GET /songs?q=…</code></dt><dd className="muted">Your songs; <code>GET /songs/ID</code> with its words and who sang it.</dd></div>
          <div><dt><code>POST /songs</code></dt><dd className="muted"><code>{'{"url": "https://youtu.be/…"}'}</code> adds a YouTube video or a song file on the web; <code>{'{"title": "…", "lyrics": "…"}'}</code> just the words.</dd></div>
          <div><dt><code>POST /nights/ID/queue</code></dt><dd className="muted"><code>{'{"song_id": 12, "singer": "Ana"}'}</code> puts a song in a night’s queue — a Shortcut on your phone.</dd></div>
          <div><dt><code>POST /nights/ID/control</code></dt><dd className="muted"><code>{'{"action": "next"}'}</code> runs the night (start, next, play, pause, skip, key, voice, volume, end) — a Stream Deck button.</dd></div>
          <div><dt><code>GET /search?q=…</code></dt><dd className="muted">Songs by title, artist or a line of their words; singers; nights.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">What was sung on a day.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const { toast } = useToast()
  const file = React.useRef<HTMLInputElement>(null)
  const bring = async (fl: File) => {
    try {
      const res = await fetch(`${V1}/import`, { method: 'POST', body: await fl.text(), headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin' })
      const j = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(j.error || res.statusText)
      toast(j.text || 'Brought in')
    } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <div className="stack loose">
      <Section title="Export" icon="archive" sub="Everything you made as one JSON file — songs with their words and settings, singers, song books, nights and who sang what — or a table of every song sung (CSV). The song files themselves are in the data folder (and the box backup).">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Sung songs as CSV</a></div>
      </Section>
      <Section title="Bring a file in" icon="upload" sub="A Pocket Karaoke export (also from another box): its songs, singers, nights and history come back as yours. The same file twice adds nothing twice. Uploaded song files need uploading again; YouTube and Pocket Music songs play straight away.">
        <button type="button" className="btn" onClick={() => file.current?.click()}><Icon name="upload" size={15} />Bring a file…</button>
        <input ref={file} type="file" hidden accept=".json,application/json" onChange={e => { const fl = e.target.files?.[0]; e.target.value = ''; if (fl) bring(fl) }} />
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-karaoke.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Songs', counts.songs, 'music'], ['Singers', counts.singers, 'users'], ['Song books', counts.books, 'book-open'], ['Nights', counts.nights, 'mic'], ['Guests', counts.guests, 'smartphone'], ['Songs sung', counts.performances, 'history']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Karaoke</h3>
          <p className="muted small">Karaoke at home: songs from uploads, YouTube and Pocket Music with timed words; key change, less voice, a score and recordings; nights where the big screen plays the queue and guests pick songs on their phones. One Go binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-karaoke" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Calendar, Contacts, Family, Email, Chat, Photos, Notes, Files, Voice, Tasks, Alerts, Blog, Weather, Bookshelf, Music, Movement, Health, Wealth, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}

function WhereThingsGo({ f, setF }: { f: SettingsT; setF: (s: SettingsT) => void }) {
  const { data: albums } = useAsync(() => f.photos_via ? api.get<{ albums: { id: string; name: string; count: number }[] }>(`${V1}/apps/photos/albums`).then(r => r.albums).catch(() => []) : Promise.resolve([]), [f.photos_via])
  const { data: habits } = useAsync(() => f.habits_via ? api.get<{ habits: { id: number; name: string }[] }>(`${V1}/apps/habits`).then(r => r.habits).catch(() => []) : Promise.resolve([]), [f.habits_via])
  const { data: chats } = useAsync(() => f.chat_via ? api.get<{ conversations: { id: string; name: string }[] }>(`${V1}/apps/chats`).then(r => r.conversations).catch(() => []) : Promise.resolve([]), [f.chat_via])
  return (
    <Section title="Where things go" icon="layers">
      <SettingRow title="Put a planned night in Pocket Calendar" help="When you make a night with a day."><Switch checked={f.calendar_on_night} disabled={!f.calendar_via} onChange={v => setF({ ...f, calendar_on_night: v })} label="Calendar" /></SettingRow>
      <SettingRow title="Photos between songs" help={f.photos_via ? 'An album from Pocket Photos on the big screen while nobody sings.' : 'Needs Pocket Photos.'}>
        <Select ariaLabel="Album" value={f.photos_album} onChange={v => setF({ ...f, photos_album: v })} placeholder="None" clearLabel="None" disabled={!f.photos_via} options={(albums || []).map(a => ({ value: a.id, label: a.name, hint: String(a.count) }))} />
      </SettingRow>
      <SettingRow title="Chat" help="The Pocket Chat conversation invitations and results go to first; empty: a note to yourself.">
        {chats?.length ? <Select ariaLabel="Chat" value={f.chat_to} onChange={v => setF({ ...f, chat_to: v })} placeholder="Me" clearLabel="Me" options={chats.map(c => ({ value: c.id, label: c.name || c.id }))} /> : <input value={f.chat_to} onChange={e => setF({ ...f, chat_to: e.target.value })} placeholder="me" disabled={!f.chat_via} />}
      </SettingRow>
      <SettingRow title="Files folder" help="Where recordings go in Pocket Files."><input value={f.files_folder} onChange={e => setF({ ...f, files_folder: e.target.value })} disabled={!f.files_via} /></SettingRow>
      <SettingRow title="Singing habit" help={f.habits_via ? 'Ticked in Pocket Habits when you practise a song for more than a minute.' : 'Needs Pocket Habits.'}>
        <Select ariaLabel="Habit" value={f.habit_id ? String(f.habit_id) : ''} onChange={v => setF({ ...f, habit_id: Number(v) || 0 })} placeholder="None" clearLabel="None" disabled={!f.habits_via} options={(habits || []).map(h => ({ value: String(h.id), label: h.name }))} />
      </SettingRow>
      <SettingRow title="A Pocket Music playlist after each night" help="Of the songs sung that you have in Pocket Music (the originals)."><Switch checked={f.music_playlist} disabled={!f.music_via} onChange={v => setF({ ...f, music_playlist: v })} label="Playlist" /></SettingRow>
    </Section>
  )
}
