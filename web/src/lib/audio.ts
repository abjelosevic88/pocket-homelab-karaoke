// The sound on the stage: the song through a voice cut (the middle of the
// stereo picture, where a lead voice sits, in the voice's range) and a key
// change (public/pitch-shifter.js), the microphone with echo through the
// speakers when wanted, the voice's notes for the pitch line and the score,
// and a recording of both. Works only on songs played from this server (a
// YouTube video's sound cannot be reached from the page).

export interface PitchFrame { t: number; midi: number | null; rms: number }

let workletLoaded: Promise<void> | null = null

export class SongEngine {
  ctx: AudioContext
  el: HTMLMediaElement
  private src: MediaElementAudioSourceNode
  private up: GainNode
  private cut: GainNode
  private shifter: AudioWorkletNode | null = null
  private out: GainNode
  private recDest: MediaStreamAudioDestinationNode
  private mic: MicInput | null = null
  private key = 0

  constructor(el: HTMLMediaElement) {
    const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
    this.ctx = new AC({ latencyHint: 'interactive' })
    this.el = el
    this.src = this.ctx.createMediaElementSource(el)
    // a mono song comes out of both sides
    this.up = this.ctx.createGain()
    this.up.channelCount = 2
    this.up.channelCountMode = 'explicit'
    this.up.channelInterpretation = 'speakers'
    this.src.connect(this.up)
    const split = this.ctx.createChannelSplitter(2)
    const merge = this.ctx.createChannelMerger(2)
    this.up.connect(split)
    split.connect(merge, 0, 0)
    split.connect(merge, 1, 1)
    // the middle: (L + R) / 2, only where a voice is (about 150 Hz – 7 kHz), taken away from both sides
    const mid = this.ctx.createGain()
    mid.gain.value = 0.5
    split.connect(mid, 0)
    split.connect(mid, 1)
    const hp = this.ctx.createBiquadFilter()
    hp.type = 'highpass'
    hp.frequency.value = 150
    const lp = this.ctx.createBiquadFilter()
    lp.type = 'lowpass'
    lp.frequency.value = 7000
    this.cut = this.ctx.createGain()
    this.cut.gain.value = 0
    mid.connect(hp).connect(lp).connect(this.cut)
    this.cut.connect(merge, 0, 0)
    this.cut.connect(merge, 0, 1)
    this.out = this.ctx.createGain()
    this.recDest = this.ctx.createMediaStreamDestination()
    this.out.connect(this.ctx.destination)
    this.out.connect(this.recDest)
    this.mergeNode = merge
    merge.connect(this.out)
  }

  private mergeNode: ChannelMergerNode

  /** Loads the key changer (once per page). */
  async ready() {
    if (this.ctx.state === 'suspended') await this.ctx.resume().catch(() => {})
    if (!this.ctx.audioWorklet) return
    try {
      if (!workletLoaded) workletLoaded = this.ctx.audioWorklet.addModule('/pitch-shifter.js')
      await workletLoaded
    } catch { workletLoaded = null; return }
    if (!this.shifter) {
      try {
        this.shifter = new AudioWorkletNode(this.ctx, 'pkar-pitch-shifter', { numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [2] })
      } catch { this.shifter = null }
      if (this.key !== 0) this.setKey(this.key)
    }
  }

  get canChangeKey() { return !!this.ctx.audioWorklet }

  /** Moves the key by semitones (−6…+6); 0 = as recorded. */
  setKey(semitones: number) {
    this.key = Math.max(-6, Math.min(6, Math.round(semitones)))
    if (!this.shifter) return
    this.mergeNode.disconnect()
    this.shifter.disconnect()
    if (this.key === 0) {
      this.mergeNode.connect(this.out)
    } else {
      this.mergeNode.connect(this.shifter).connect(this.out)
      this.shifter.parameters.get('ratio')!.setValueAtTime(Math.pow(2, this.key / 12), this.ctx.currentTime)
    }
  }

  /** How much of the lead voice to take away, 0…100. */
  setVoiceCut(pct: number) {
    this.cut.gain.setTargetAtTime(-Math.max(0, Math.min(100, pct)) / 100 * 1.6, this.ctx.currentTime, 0.05)
  }

  setVolume(pct: number) {
    this.out.gain.setTargetAtTime(Math.max(0, Math.min(100, pct)) / 100, this.ctx.currentTime, 0.05)
  }

  /** Starts listening to the microphone (monitor = the voice through the speakers, with echo). */
  async startMic(monitor: boolean, echo: number): Promise<MicInput> {
    if (this.mic) { this.mic.setMonitor(monitor, echo); return this.mic }
    this.mic = await MicInput.open(this.ctx, monitor, echo)
    this.mic.output.connect(this.recDest)
    return this.mic
  }

  stopMic() { this.mic?.close(); this.mic = null }
  get micOn() { return !!this.mic }
  get micInput() { return this.mic }

  /** Records the song with the voice (the browser's own format: WebM/Opus, or MP4 on Safari). */
  record(): Recorder | null {
    if (typeof MediaRecorder === 'undefined') return null
    const types = ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4', 'audio/ogg']
    const type = types.find(t => MediaRecorder.isTypeSupported(t)) || ''
    try { return new Recorder(new MediaRecorder(this.recDest.stream, type ? { mimeType: type } : undefined)) } catch { return null }
  }

  close() {
    this.stopMic()
    this.ctx.close().catch(() => {})
  }
}

/** The microphone: the notes being sung, and the voice through the speakers. */
export class MicInput {
  private ctx: AudioContext
  private stream: MediaStream
  private analyser: AnalyserNode
  private buf: Float32Array<ArrayBuffer>
  private dry: GainNode
  private wet: GainNode
  private delay: DelayNode
  private fb: GainNode
  output: GainNode

  private constructor(ctx: AudioContext, stream: MediaStream) {
    this.ctx = ctx
    this.stream = stream
    const src = ctx.createMediaStreamSource(stream)
    this.analyser = ctx.createAnalyser()
    this.analyser.fftSize = 2048
    this.buf = new Float32Array(this.analyser.fftSize)
    src.connect(this.analyser)
    // through the speakers: dry + a short echo
    this.output = ctx.createGain()
    this.dry = ctx.createGain()
    this.wet = ctx.createGain()
    this.delay = ctx.createDelay(1)
    this.delay.delayTime.value = 0.16
    this.fb = ctx.createGain()
    this.fb.gain.value = 0.32
    src.connect(this.dry).connect(this.output)
    src.connect(this.delay)
    this.delay.connect(this.fb).connect(this.delay)
    this.delay.connect(this.wet).connect(this.output)
    this.monitorGain = ctx.createGain()
    this.output.connect(this.monitorGain).connect(ctx.destination)
  }

  private monitorGain: GainNode

  static async open(ctx: AudioContext, monitor: boolean, echo: number): Promise<MicInput> {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: !monitor, noiseSuppression: false, autoGainControl: true } })
    const m = new MicInput(ctx, stream)
    m.setMonitor(monitor, echo)
    return m
  }

  setMonitor(on: boolean, echo: number) {
    const e = Math.max(0, Math.min(100, echo)) / 100
    this.monitorGain.gain.value = on ? 1 : 0
    this.dry.gain.value = 1
    this.wet.gain.value = e * 0.7
    this.fb.gain.value = 0.15 + e * 0.35
  }

  /** The note being sung now (MIDI number with cents as fraction), or null when nobody sings. */
  frame(t: number): PitchFrame {
    this.analyser.getFloatTimeDomainData(this.buf)
    const { freq, clarity, rms } = detectPitch(this.buf, this.ctx.sampleRate)
    const midi = freq && clarity > 0.86 && rms > 0.012 ? 69 + 12 * Math.log2(freq / 440) : null
    return { t, midi, rms }
  }

  close() {
    this.stream.getTracks().forEach(t => t.stop())
    this.output.disconnect()
    this.monitorGain.disconnect()
  }
}

/** Autocorrelation pitch finding (normalised, with parabolic refinement). */
export function detectPitch(buf: Float32Array, rate: number): { freq: number | null; clarity: number; rms: number } {
  const n = buf.length
  let rms = 0
  for (let i = 0; i < n; i++) rms += buf[i] * buf[i]
  rms = Math.sqrt(rms / n)
  if (rms < 0.006) return { freq: null, clarity: 0, rms }
  const minLag = Math.floor(rate / 1100), maxLag = Math.min(Math.floor(rate / 75), n >> 1)
  let best = -1, bestLag = -1
  const corr = new Float32Array(maxLag + 2)
  for (let lag = minLag; lag <= maxLag + 1; lag++) {
    let sum = 0, e1 = 0, e2 = 0
    for (let i = 0; i + lag < n; i += 2) {
      const a = buf[i], b = buf[i + lag]
      sum += a * b; e1 += a * a; e2 += b * b
    }
    const c = sum / Math.sqrt(e1 * e2 + 1e-12)
    corr[lag] = c
  }
  // the first strong peak (not just the highest: octave errors)
  let peakMax = 0
  for (let lag = minLag + 1; lag <= maxLag; lag++) peakMax = Math.max(peakMax, corr[lag])
  for (let lag = minLag + 1; lag <= maxLag; lag++) {
    if (corr[lag] > corr[lag - 1] && corr[lag] >= corr[lag + 1] && corr[lag] > 0.9 * peakMax) { best = corr[lag]; bestLag = lag; break }
  }
  if (bestLag < 0) return { freq: null, clarity: 0, rms }
  const a = corr[bestLag - 1], b = corr[bestLag], c = corr[bestLag + 1]
  const shift = (a - c) / (2 * (a - 2 * b + c) || 1)
  const lag = bestLag + (Number.isFinite(shift) ? Math.max(-1, Math.min(1, shift)) : 0)
  return { freq: rate / lag, clarity: best, rms }
}

/** Keeps a performance's score: how much of the words' time a voice was
 *  heard (on time), and how much of the singing held a note (steady). For
 *  fun — it does not know the song's melody. */
export class Scorer {
  private active = 0
  private voicedActive = 0
  private voiced = 0
  private steady = 0
  private last: number | null = null
  trail: PitchFrame[] = []

  add(f: PitchFrame, singingTime: boolean) {
    if (singingTime) {
      this.active++
      if (f.midi !== null) this.voicedActive++
    }
    if (f.midi !== null) {
      this.voiced++
      if (this.last !== null && Math.abs(f.midi - this.last) < 0.5) this.steady++
    }
    this.last = f.midi
    this.trail.push(f)
    while (this.trail.length && f.t - this.trail[0].t > 6000) this.trail.shift()
  }

  get heard() { return this.voiced > 8 }

  result(): { score: number; on_time: number; steady: number } | null {
    if (this.active < 20 || !this.heard) return null
    const onTime = Math.min(1, this.voicedActive / this.active / 0.75) // nobody sings every moment of a line
    const steady = this.voiced ? this.steady / this.voiced : 0
    const raw = 0.62 * onTime + 0.38 * Math.min(1, steady / 0.8)
    return { score: Math.round(Math.min(100, 25 + 75 * Math.sqrt(raw))), on_time: Math.round(onTime * 100), steady: Math.round(Math.min(1, steady / 0.8) * 100) }
  }
}

export class Recorder {
  private chunks: Blob[] = []
  readonly rec: MediaRecorder
  constructor(rec: MediaRecorder) {
    this.rec = rec
    rec.ondataavailable = e => { if (e.data.size) this.chunks.push(e.data) }
  }
  start() { this.chunks = []; this.rec.start(1000) }
  stop(): Promise<Blob> {
    return new Promise(res => {
      if (this.rec.state === 'inactive') { res(new Blob(this.chunks, { type: this.rec.mimeType })); return }
      this.rec.onstop = () => res(new Blob(this.chunks, { type: (this.rec.mimeType || 'audio/webm').split(';')[0] }))
      this.rec.stop()
    })
  }
}

export const NOTE_NAMES = ['C', 'C♯', 'D', 'E♭', 'E', 'F', 'F♯', 'G', 'A♭', 'A', 'B♭', 'B']
export function noteName(midi: number): string { const m = Math.round(midi); return NOTE_NAMES[((m % 12) + 12) % 12] + (Math.floor(m / 12) - 1) }

/** Keeps the screen on while singing (where the browser can). */
export async function wakeLock(): Promise<() => void> {
  try {
    const nav = navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<{ release: () => Promise<void> }> } }
    const l = await nav.wakeLock?.request('screen')
    return () => { l?.release().catch(() => {}) }
  } catch { return () => {} }
}
