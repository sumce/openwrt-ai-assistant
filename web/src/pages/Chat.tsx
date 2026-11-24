import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Button, Empty, Input, Message, Tag } from '@arco-design/web-react'
import { IconSend } from '@arco-design/web-react/icon'
import { api } from '../api'
import { useApp } from '../store'
import { RunStream } from '../components/RunStream'
import { EntryItem, LiveSegItem, mergeEvent, type LiveSeg } from '../components/MessageItem'
import type { Entry, RunEvent, RunSnapshot } from '../types'

const { TextArea } = Input

const HINTS = [
  '查看路由器状态',
  '看看最近的系统日志',
  '列出当前在线设备',
  '把防火墙 WAN 入站改成 ACCEPT（会演示危险操作审批）',
]

export default function Chat() {
  const { conversations, currentConvId, setCurrentConv, loadConversations } = useApp()

  const [entries, setEntries] = useState<Entry[]>([])
  const [segs, setSegs] = useState<LiveSeg[]>([])
  const [runId, setRunId] = useState<string | null>(null)
  const [runState, setRunState] = useState<string>('')
  const [input, setInput] = useState('')
  const [approving, setApproving] = useState(false)

  const streamRef = useRef<RunStream | null>(null)
  const convIdRef = useRef<string | null>(currentConvId)
  const runIdRef = useRef<string | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const stickBottom = useRef(true)

  convIdRef.current = currentConvId
  runIdRef.current = runId

  const reload = async (convId: string | null) => {
    if (!convId) {
      setEntries([])
      return
    }
    try {
      setEntries(await api.messages(convId))
    } catch (e: any) {
      Message.error(e.message || '加载会话失败')
    }
  }

  const finishRun = () => {
    setSegs([])
    setRunId(null)
    setRunState('')
    reload(convIdRef.current)
    loadConversations()
  }

  // handler is created once; it only touches refs and setState updaters so
  // there are no stale-closure issues.
  if (!streamRef.current) {
    streamRef.current = new RunStream((e: RunEvent) => {
      switch (e.type) {
        case 'text_delta':
        case 'tool_start':
        case 'tool_result':
        case 'approval_request':
          setSegs((prev) => mergeEvent(prev, e))
          if (e.type === 'approval_request') setRunState('waiting_approval')
          break
        case 'state':
          setRunState(e.state || '')
          break
        case 'error':
          Message.error(e.error || '任务失败')
          break
        case 'done':
          finishRun()
          break
      }
    })
  }

  // Selecting a conversation: load transcript, reattach any active run.
  const selectConv = async (id: string | null) => {
    setCurrentConv(id)
    setSegs([])
    setRunId(null)
    setRunState('')
    if (!id) {
      setEntries([])
      return
    }
    await reload(id)
    try {
      const active: RunSnapshot[] = await api.activeRuns()
      const match = active.find((a) => a.conversation_id === id)
      if (match) {
        setRunId(match.id)
        setRunState(match.state)
        // Seed the approval card from the snapshot (covers restart cases
        // where the SSE replay buffer is empty); dedupe happens in merge.
        if (match.pending && match.state === 'waiting_approval') {
          setSegs((prev) =>
            mergeEvent(prev, { seq: -1, type: 'approval_request', pending: match.pending! }),
          )
        }
        streamRef.current!.resume(match.id, 0)
      }
    } catch {
      /* not authed / no active run */
    }
  }

  // Reattach on first mount (e.g. browser refresh mid-run).
  useEffect(() => {
    (async () => {
      try {
        const active: RunSnapshot[] = await api.activeRuns()
        if (active.length === 0) return
        const match =
          active.find((a) => a.conversation_id === currentConvId) || active[active.length - 1]
        setCurrentConv(match.conversation_id)
        setEntries(await api.messages(match.conversation_id))
        setRunId(match.id)
        setRunState(match.state)
        convIdRef.current = match.conversation_id
        if (match.pending && match.state === 'waiting_approval') {
          setSegs((prev) =>
            mergeEvent(prev, { seq: -1, type: 'approval_request', pending: match.pending! }),
          )
        }
        streamRef.current!.resume(match.id, 0)
      } catch {
        /* ignore */
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const send = async () => {
    const text = input.trim()
    if (!text || runIdRef.current) return
    setInput('')
    const optimistic: Entry = { t: 'user', text, ts: Math.floor(Date.now() / 1000) }
    setEntries((prev) => [...prev, optimistic])
    try {
      const res = await api.chat(currentConvId, text)
      convIdRef.current = res.conversation_id
      if (!currentConvId) {
        setCurrentConv(res.conversation_id)
        loadConversations()
      }
      setSegs([])
      setRunId(res.run_id)
      setRunState('running')
      streamRef.current!.start(res.run_id)
    } catch (e: any) {
      Message.error(e.message || '发送失败')
      setEntries((prev) => prev.filter((x) => x !== optimistic))
      setInput(text)
    }
  }

  const approve = async (v: boolean) => {
    if (!runIdRef.current) return
    setApproving(true)
    try {
      await api.approve(runIdRef.current, v)
      setSegs((prev) =>
        prev.map((s) => (s.kind === 'approval' && s.decided === null ? { ...s, decided: v } : s)),
      )
      setRunState('running')
    } catch (e: any) {
      Message.error(e.message || '操作失败')
    } finally {
      setApproving(false)
    }
  }

  const cancel = async () => {
    if (!runIdRef.current) return
    try {
      await api.cancelRun(runIdRef.current)
      Message.info('已请求停止')
    } catch (e: any) {
      Message.error(e.message || '停止失败')
    }
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const onScroll = () => {
    const el = scrollRef.current
    if (!el) return
    stickBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 160
  }

  useEffect(() => {
    const el = scrollRef.current
    if (el && stickBottom.current) el.scrollTop = el.scrollHeight
  }, [entries, segs])

  const conv = conversations.find((c) => c.id === currentConvId)
  const running = !!runId

  return (
    <div className="chat">
      <div className="chat-header">
        <span className="chat-title">{conv ? conv.title : '新会话'}</span>
        {runState === 'waiting_approval' && <Tag color="orange">等待确认</Tag>}
        {running && runState === 'running' && <Tag color="arcoblue">执行中</Tag>}
      </div>

      <div className="chat-scroll" ref={scrollRef} onScroll={onScroll}>
        {entries.length === 0 && segs.length === 0 ? (
          <div className="chat-empty">
            <Empty description="和你的路由器聊聊吧" />
            <div className="hints">
              {HINTS.map((h) => (
                <span key={h} className="hint-chip" onClick={() => setInput(h)}>
                  {h}
                </span>
              ))}
            </div>
          </div>
        ) : (
          <>
            {entries.map((e, i) => (
              <EntryItem key={`e${i}`} e={e} />
            ))}
            {segs.map((s, i) => (
              <LiveSegItem key={`s${i}`} seg={s} onApprove={approve} busy={approving} />
            ))}
          </>
        )}
      </div>

      <div className="composer">
        <TextArea
          placeholder={running ? '任务执行中…' : '输入消息，Enter 发送，Shift+Enter 换行'}
          autoSize={{ minRows: 1, maxRows: 5 }}
          value={input}
          onChange={setInput}
          onKeyDown={onKeyDown}
          disabled={running}
        />
        <div className="composer-actions">
          {running ? (
            <Button status="warning" onClick={cancel}>
              停止
            </Button>
          ) : (
            <Button type="primary" icon={<IconSend />} disabled={!input.trim()} onClick={send}>
              发送
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
