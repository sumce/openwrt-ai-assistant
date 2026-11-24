import { useState } from 'react'
import { Alert, Button, Spin, Tag } from '@arco-design/web-react'
import { IconCaretRight, IconCaretDown, IconCheckCircleFill, IconCloseCircleFill, IconExclamationCircleFill } from '@arco-design/web-react/icon'
import Markdown from './Markdown'
import type { Entry, PendingAction, RunEvent } from '../types'

// ---- canonical transcript entry ----

export function EntryItem({ e }: { e: Entry }) {
  switch (e.t) {
    case 'user':
      return (
        <div className="msg msg-user">
          <div className="bubble-user">{e.text}</div>
        </div>
      )
    case 'assistant':
      return (
        <div className="msg msg-ai">
          <Markdown text={e.text || ''} />
        </div>
      )
    case 'tool':
      return <ToolCard tool={e.tool || ''} desc={e.desc || ''} result={e.result} ok={e.result ? !isFailResult(e.result) : undefined} />
    case 'approval':
      return (
        <div className="approval-log">
          {e.approved ? (
            <Tag color="green" size="small">已批准</Tag>
          ) : (
            <Tag color="red" size="small">已拒绝</Tag>
          )}
          <span className="approval-desc">{e.desc}</span>
        </div>
      )
    case 'error':
      return <Alert type="error" content={e.text || '出错了'} style={{ margin: '8px 0' }} />
    default:
      return null
  }
}

function isFailResult(result: string) {
  return result.startsWith('执行失败') || result.startsWith('备份失败')
}

export function ToolCard({ tool, desc, result, ok }: { tool: string; desc: string; result?: string; ok?: boolean }) {
  const [open, setOpen] = useState(false)
  const running = ok === undefined
  const expandable = !!result
  return (
    <div className={`tool-card${ok === false ? ' tool-fail' : ''}`}>
      <div className="tool-head" onClick={() => expandable && setOpen(!open)}>
        <span className="tool-icon">
          {running ? (
            <Spin size={14} />
          ) : ok ? (
            <IconCheckCircleFill style={{ color: 'var(--color-success-6)', fontSize: 14 }} />
          ) : (
            <IconCloseCircleFill style={{ color: 'var(--color-danger-6)', fontSize: 14 }} />
          )}
        </span>
        <Tag size="small" color="arcoblue">{tool}</Tag>
        <span className="tool-desc">{desc}{running ? ' …' : ''}</span>
        {expandable && (open ? <IconCaretDown className="tool-caret" /> : <IconCaretRight className="tool-caret" />)}
      </div>
      {open && result && <pre className="tool-result">{result}</pre>}
    </div>
  )
}

// ---- live (in-flight) segments for the running turn ----

export type LiveSeg =
  | { kind: 'text'; text: string }
  | { kind: 'tool'; tool: string; desc: string; level: string; result?: string; ok?: boolean }
  | { kind: 'approval'; pending: PendingAction; decided: boolean | null }

export function LiveSegItem({ seg, onApprove, busy }: { seg: LiveSeg; onApprove: (v: boolean) => void; busy: boolean }) {
  if (seg.kind === 'text') {
    return (
      <div className="msg msg-ai">
        <Markdown text={seg.text} />
      </div>
    )
  }
  if (seg.kind === 'tool') {
    return <ToolCard tool={seg.tool} desc={seg.desc} result={seg.result} ok={seg.ok} />
  }
  const d = seg.decided
  return (
    <div className="approval-card">
      <div className="approval-head">
        <IconExclamationCircleFill style={{ color: 'var(--color-warning-6)' }} />
        <b>需要你确认</b>
        <Tag size="small" color={seg.pending.level === 'dangerous' ? 'red' : 'orange'}>
          {seg.pending.level === 'dangerous' ? '危险操作' : '写操作'}
        </Tag>
      </div>
      <div className="approval-body">{seg.pending.description}</div>
      {seg.pending.configs && seg.pending.configs.length > 0 && (
        <div className="approval-configs">
          影响配置：
          {seg.pending.configs.map((c) => (
            <Tag key={c} size="small">{c}</Tag>
          ))}
        </div>
      )}
      {seg.pending.level === 'dangerous' && (
        <div className="approval-note">执行前会自动备份当前配置，可在 设置 → 备份 中一键回滚。</div>
      )}
      {d === null ? (
        <div className="approval-actions">
          <Button type="primary" status="success" size="small" loading={busy} onClick={() => onApprove(true)}>
            批准执行
          </Button>
          <Button status="danger" size="small" disabled={busy} onClick={() => onApprove(false)}>
            拒绝
          </Button>
        </div>
      ) : (
        <div className="approval-actions">
          <Tag color={d ? 'green' : 'red'}>{d ? '已批准，执行中…' : '已拒绝'}</Tag>
        </div>
      )}
    </div>
  )
}

export function mergeEvent(segs: LiveSeg[], e: RunEvent): LiveSeg[] {
  switch (e.type) {
    case 'text_delta': {
      const last = segs[segs.length - 1]
      if (last && last.kind === 'text') {
        const next = [...segs]
        next[next.length - 1] = { kind: 'text', text: last.text + (e.text || '') }
        return next
      }
      return [...segs, { kind: 'text', text: e.text || '' }]
    }
    case 'tool_start':
      return [...segs, { kind: 'tool', tool: e.tool || '', desc: e.desc || '', level: e.level || 'read' }]
    case 'tool_result': {
      const idx = lastPendingTool(segs)
      const next = [...segs]
      if (idx >= 0) {
        const t = next[idx]
        if (t.kind === 'tool') {
          next[idx] = { ...t, result: e.result, ok: e.ok }
          return next
        }
      }
      return [...segs, { kind: 'tool', tool: e.tool || '', desc: e.desc || '', level: e.level || 'read', result: e.result, ok: e.ok }]
    }
    case 'approval_request':
      return [...segs, { kind: 'approval', pending: e.pending!, decided: null }]
    default:
      return segs
  }
}

function lastPendingTool(segs: LiveSeg[]): number {
  for (let i = segs.length - 1; i >= 0; i--) {
    const s = segs[i]
    if (s.kind === 'tool' && s.ok === undefined) return i
  }
  return -1
}
