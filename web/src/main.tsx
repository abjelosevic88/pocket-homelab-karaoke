import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/karaoke.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Home from './pages/Home'
import Songs from './pages/Songs'
import SongPage from './pages/SongPage'
import Sing from './pages/Sing'
import Nights from './pages/Nights'
import NightPage from './pages/NightPage'
import Screen from './pages/Screen'
import Join, { EnterCode } from './pages/Join'
import Singers, { SingerPage } from './pages/Singers'
import Books from './pages/Books'
import History from './pages/History'
import Settings from './pages/Settings'
import { DataProvider } from './lib/data'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/j" element={<ErrorBoundary where="join"><EnterCode /></ErrorBoundary>} />
      <Route path="/j/:code" element={<ErrorBoundary where="join"><Join /></ErrorBoundary>} />
      <Route path="/screen/:key" element={<ErrorBoundary where="screen"><Screen /></ErrorBoundary>} />
      <Route path="/sing/:id" element={<Gate><ErrorBoundary where="sing"><DataProvider><Sing /></DataProvider></ErrorBoundary></Gate>} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><DataProvider><Settings /></DataProvider></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="karaoke"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Home />} />
        <Route path="/songs" element={<Songs />} />
        <Route path="/songs/:id" element={<SongPage />} />
        <Route path="/nights" element={<Nights />} />
        <Route path="/nights/:id" element={<NightPage />} />
        <Route path="/singers" element={<Singers />} />
        <Route path="/singers/:id" element={<SingerPage />} />
        <Route path="/books" element={<Books />} />
        <Route path="/history" element={<History />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
