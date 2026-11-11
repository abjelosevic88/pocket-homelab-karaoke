// CD+G: the graphics of classic karaoke discs (an MP3 with a .cdg beside
// it). 300 packets a second of 24 bytes; a 300×216 picture in 16 colours,
// drawn in 6×12 tiles. We play the packets up to the song's time onto a
// canvas.

const W = 300, H = 216
const PACKETS_PER_SEC = 300

export class CDGPlayer {
  private data: Uint8Array
  private pixels = new Uint8Array(W * H)
  private palette = new Uint32Array(16)
  private border = 0
  private hOffset = 0
  private vOffset = 0
  private pos = 0 // packets done
  private img: ImageData
  private ctx: CanvasRenderingContext2D
  private dirty = true

  constructor(data: ArrayBuffer, canvas: HTMLCanvasElement) {
    this.data = new Uint8Array(data)
    canvas.width = W - 12
    canvas.height = H - 24
    this.ctx = canvas.getContext('2d')!
    this.img = this.ctx.createImageData(W - 12, H - 24)
    this.reset()
  }

  private reset() {
    this.pixels.fill(0)
    this.palette.fill(0xff000000)
    this.border = 0
    this.hOffset = this.vOffset = 0
    this.pos = 0
    this.dirty = true
  }

  /** Plays the graphics up to t milliseconds and draws them. */
  render(t: number) {
    const target = Math.min(Math.floor((t / 1000) * PACKETS_PER_SEC), Math.floor(this.data.length / 24))
    if (target < this.pos) this.reset()
    while (this.pos < target) {
      this.packet(this.pos * 24)
      this.pos++
    }
    if (this.dirty) this.draw()
  }

  private packet(o: number) {
    const d = this.data
    if ((d[o] & 0x3f) !== 9) return
    const inst = d[o + 1] & 0x3f
    const data = d.subarray(o + 4, o + 20)
    switch (inst) {
      case 1: { // memory preset
        if ((data[1] & 0x0f) === 0) this.pixels.fill(data[0] & 0x0f)
        this.dirty = true
        break
      }
      case 2: { // border preset
        this.border = data[0] & 0x0f
        this.dirty = true
        break
      }
      case 6: case 38: { // tile block (normal / xor)
        const c0 = data[0] & 0x0f, c1 = data[1] & 0x0f
        const row = data[2] & 0x1f, col = data[3] & 0x3f
        const x0 = col * 6, y0 = row * 12
        if (x0 + 6 > W || y0 + 12 > H) break
        for (let y = 0; y < 12; y++) {
          const bits = data[4 + y] & 0x3f
          for (let x = 0; x < 6; x++) {
            const on = (bits >> (5 - x)) & 1
            const i = (y0 + y) * W + x0 + x
            const c = on ? c1 : c0
            this.pixels[i] = inst === 38 ? (this.pixels[i] ^ c) & 0x0f : c
          }
        }
        this.dirty = true
        break
      }
      case 20: case 24: { // scroll preset / copy
        const color = data[0] & 0x0f
        const hCmd = (data[1] & 0x30) >> 4, hOff = data[1] & 0x07
        const vCmd = (data[2] & 0x30) >> 4, vOff = data[2] & 0x0f
        this.hOffset = Math.min(hOff, 5)
        this.vOffset = Math.min(vOff, 11)
        const dx = hCmd === 2 ? 6 : hCmd === 1 ? -6 : 0
        const dy = vCmd === 2 ? 12 : vCmd === 1 ? -12 : 0
        if (dx || dy) {
          const old = this.pixels.slice()
          for (let y = 0; y < H; y++) {
            for (let x = 0; x < W; x++) {
              let sx = x - dx, sy = y - dy
              let c = color
              if (inst === 24) { sx = (sx + W) % W; sy = (sy + H) % H; c = old[sy * W + sx] }
              else if (sx >= 0 && sx < W && sy >= 0 && sy < H) c = old[sy * W + sx]
              this.pixels[y * W + x] = c
            }
          }
        }
        this.dirty = true
        break
      }
      case 30: case 31: { // the colour table, low (0–7) or high (8–15)
        const base = inst === 30 ? 0 : 8
        for (let i = 0; i < 8; i++) {
          const hi = data[2 * i] & 0x3f, lo = data[2 * i + 1] & 0x3f
          const r = (hi >> 2) & 0x0f
          const g = ((hi & 0x03) << 2) | ((lo >> 4) & 0x03)
          const b = lo & 0x0f
          this.palette[base + i] = (0xff << 24) | ((b * 17) << 16) | ((g * 17) << 8) | (r * 17)
        }
        this.dirty = true
        break
      }
    }
  }

  private draw() {
    const out = new Uint32Array(this.img.data.buffer)
    const w = W - 12, h = H - 24
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const sx = x + 6 + this.hOffset, sy = y + 12 + this.vOffset
        const c = sx < W && sy < H ? this.pixels[sy * W + sx] : this.border
        out[y * w + x] = this.palette[c]
      }
    }
    this.ctx.putImageData(this.img, 0, 0)
    this.dirty = false
  }

  /** The border colour, for the area around the picture. */
  get borderColour(): string {
    const v = this.palette[this.border]
    return `rgb(${v & 0xff}, ${(v >> 8) & 0xff}, ${(v >> 16) & 0xff})`
  }
}
