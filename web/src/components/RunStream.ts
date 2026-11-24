import type { RunEvent } from '../types'

// RunStream subscribes to one run's SSE event stream, surviving reconnects
// (Wi-Fi switches kill SSE constantly) by replaying from the last seen seq.
export class RunStream {
  private es: EventSource | null = null
  private runId = ''
  private lastSeq = 0
  private stopped = true
  private timer: ReturnType<typeof setTimeout> | null = null

  constructor(private onEvent: (e: RunEvent) => void) {}

  start(runId: string) {
    this.resume(runId, 0)
  }

  resume(runId: string, after: number) {
    this.stop()
    this.runId = runId
    this.lastSeq = after
    this.stopped = false
    this.open()
  }

  private open() {
    const es = new EventSource(`/api/runs/${this.runId}/events?after=${this.lastSeq}`)
    this.es = es
    es.onmessage = (ev) => {
      try {
        const e: RunEvent = JSON.parse(ev.data)
        if (e.seq > this.lastSeq) this.lastSeq = e.seq
        this.onEvent(e)
        if (e.type === 'done') this.stop()
      } catch {
        /* ignore malformed */
      }
    }
    es.onerror = () => {
      es.close()
      this.es = null
      if (this.stopped) return
      this.timer = setTimeout(() => {
        if (!this.stopped) this.open()
      }, 1200)
    }
  }

  stop() {
    this.stopped = true
    if (this.timer) clearTimeout(this.timer)
    this.es?.close()
    this.es = null
  }
}
