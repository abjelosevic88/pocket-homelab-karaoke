// Pocket Karaoke: changes the key of a song as it plays (an AudioWorklet).
// Two reading heads move through a short delay line at a different speed
// than the sound is written, each faded in and out, half a grain apart —
// a granular pitch shifter: the tempo stays, the key moves.
class PitchShifter extends AudioWorkletProcessor {
  static get parameterDescriptors() {
    return [{ name: 'ratio', defaultValue: 1, minValue: 0.5, maxValue: 2, automationRate: 'k-rate' }]
  }

  constructor() {
    super()
    this.size = 1 << 15
    this.buf = [new Float32Array(this.size), new Float32Array(this.size)]
    this.w = 0
    this.phase = 0
    this.grain = Math.round(sampleRate * 0.07)
  }

  read(ch, delay) {
    const pos = this.w - delay
    const i = Math.floor(pos)
    const f = pos - i
    const b = this.buf[ch]
    const a = b[(i + this.size) & (this.size - 1)]
    const c = b[(i + 1 + this.size) & (this.size - 1)]
    return a + (c - a) * f
  }

  process(inputs, outputs, params) {
    const inp = inputs[0]
    const out = outputs[0]
    if (!out || !out.length) return true
    const ratio = params.ratio[0]
    const n = out[0].length
    const g = this.grain
    const mask = this.size - 1
    if (Math.abs(ratio - 1) < 0.001) {
      for (let ch = 0; ch < out.length; ch++) {
        const src = inp && (inp[ch] || inp[0])
        for (let i = 0; i < n; i++) {
          const x = src ? src[i] : 0
          if (ch < 2) this.buf[ch][(this.w + i) & mask] = x
          out[ch][i] = x
        }
      }
      this.w = (this.w + n) & mask
      return true
    }
    for (let i = 0; i < n; i++) {
      for (let ch = 0; ch < 2; ch++) {
        const src = inp && (inp[ch] || inp[0])
        this.buf[ch][this.w] = src ? src[i] : 0
      }
      this.phase += (1 - ratio) / g
      if (this.phase >= 1) this.phase -= 1
      if (this.phase < 0) this.phase += 1
      const p1 = this.phase
      const p2 = (p1 + 0.5) % 1
      const w1 = 0.5 - 0.5 * Math.cos(2 * Math.PI * p1)
      const w2 = 1 - w1
      for (let ch = 0; ch < out.length; ch++) {
        const c = Math.min(ch, 1)
        out[ch][i] = this.read(c, p1 * g + 2) * w1 + this.read(c, p2 * g + 2) * w2
      }
      this.w = (this.w + 1) & mask
    }
    return true
  }
}

registerProcessor('pkar-pitch-shifter', PitchShifter)
