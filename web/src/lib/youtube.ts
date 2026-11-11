// YouTube's own player (the IFrame API), loaded once when a YouTube song plays.

export interface YTPlayer {
  playVideo(): void
  pauseVideo(): void
  stopVideo(): void
  seekTo(s: number, allowSeekAhead: boolean): void
  getCurrentTime(): number
  getDuration(): number
  setVolume(v: number): void
  destroy(): void
}

interface YTNamespace {
  Player: new (el: HTMLElement, opts: Record<string, unknown>) => YTPlayer
}

let loading: Promise<YTNamespace> | null = null

export function loadYouTube(): Promise<YTNamespace> {
  const w = window as unknown as { YT?: YTNamespace & { loaded?: number }; onYouTubeIframeAPIReady?: () => void }
  if (w.YT?.Player) return Promise.resolve(w.YT)
  if (loading) return loading
  loading = new Promise((resolve, reject) => {
    const prev = w.onYouTubeIframeAPIReady
    w.onYouTubeIframeAPIReady = () => { prev?.(); resolve(w.YT!) }
    const s = document.createElement('script')
    s.src = 'https://www.youtube.com/iframe_api'
    s.async = true
    s.onerror = () => { loading = null; reject(new Error('YouTube did not load')) }
    document.head.appendChild(s)
  })
  return loading
}
