// Minimal hand-rolled UI primitives — no component library.
import { ReactNode, useEffect, useRef, useState } from 'react'
import { create } from 'zustand'

// ---------- icons (feather-style inline SVG) ----------

function I({ children, size = 16, className }: { children: ReactNode; size?: number; className?: string }) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      {children}
    </svg>
  )
}

export const IconPlus = (p: { size?: number }) => (
  <I {...p}><path d="M12 5v14M5 12h14" /></I>
)
export const IconSend = (p: { size?: number }) => (
  <I {...p}><path d="M22 2L11 13M22 2l-7 20-4-9-9-4 20-7z" /></I>
)
export const IconStop = (p: { size?: number }) => (
  <I {...p}><rect x="6" y="6" width="12" height="12" rx="2" /></I>
)
export const IconTrash = (p: { size?: number }) => (
  <I {...p}><path d="M3 6h18M8 6V4h8v2m1 0v14a2 2 0 01-2 2H9a2 2 0 01-2-2V6M10 11v6M14 11v6" /></I>
)
export const IconChevronDown = (p: { size?: number }) => <I {...p}><path d="M6 9l6 6 6-6" /></I>
export const IconChevronRight = (p: { size?: number }) => <I {...p}><path d="M9 6l6 6-6 6" /></I>
export const IconChevronLeft = (p: { size?: number }) => <I {...p}><path d="M15 6l-6 6 6 6" /></I>
export const IconCheck = (p: { size?: number }) => <I {...p}><path d="M20 6L9 17l-5-5" /></I>
export const IconCheckCircle = (p: { size?: number }) => (
  <I {...p}><path d="M22 11.08V12a10 10 0 11-5.93-9.14" /><path d="M22 4L12 14.01l-3-3" /></I>
)
export const IconXCircle = (p: { size?: number }) => (
  <I {...p}><circle cx="12" cy="12" r="10" /><path d="M15 9l-6 6M9 9l6 6" /></I>
)
export const IconX = (p: { size?: number }) => <I {...p}><path d="M18 6L6 18M6 6l12 12" /></I>
export const IconWarning = (p: { size?: number }) => (
  <I {...p}>
    <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
    <path d="M12 9v4M12 17h.01" />
  </I>
)
export const IconSettings = (p: { size?: number }) => (
  <I {...p}>
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 01-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09a1.65 1.65 0 00-1-1.51 1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09a1.65 1.65 0 001.51-1 1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06a1.65 1.65 0 001.82.33h0a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51h0a1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82v0a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z" />
  </I>
)
export const IconLogout = (p: { size?: number }) => (
  <I {...p}><path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9" /></I>
)
export const IconRefresh = (p: { size?: number }) => (
  <I {...p}><path d="M23 4v6h-6M1 20v-6h6" /><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" /></I>
)
export const IconEye = (p: { size?: number }) => (
  <I {...p}><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></I>
)
export const IconEyeOff = (p: { size?: number }) => (
  <I {...p}>
    <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24" />
    <path d="M1 1l22 22" />
  </I>
)
// sidebar collapse / expand (panel icon)
export const IconPanel = (p: { size?: number }) => (
  <I {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18" /></I>
)

// ---------- toasts ----------

type ToastType = 'info' | 'success' | 'error' | 'warning'
interface ToastItem { id: number; type: ToastType; text: string }
let toastSeq = 0

const useToastStore = create<{ items: ToastItem[]; push: (t: ToastItem) => void; remove: (id: number) => void }>((set) => ({
  items: [],
  push: (t) => set((s) => ({ items: [...s.items, t] })),
  remove: (id) => set((s) => ({ items: s.items.filter((x) => x.id !== id) })),
}))

export const Message = {
  info(text: string) { fire('info', text) },
  success(text: string) { fire('success', text) },
  error(text: string) { fire('error', text) },
  warning(text: string) { fire('warning', text) },
}

function fire(type: ToastType, text: string) {
  const id = ++toastSeq
  useToastStore.getState().push({ id, type, text })
  setTimeout(() => useToastStore.getState().remove(id), 3600)
}

export function ToastHost() {
  const { items, remove } = useToastStore()
  return (
    <div className="toast-host">
      {items.map((t) => (
        <div key={t.id} className={`toast toast-${t.type}`} onClick={() => remove(t.id)}>
          {t.type === 'success' && <IconCheckCircle size={14} />}
          {t.type === 'error' && <IconXCircle size={14} />}
          {t.type === 'warning' && <IconWarning size={14} />}
          {t.type === 'info' && <span className="toast-dot" />}
          <span>{t.text}</span>
        </div>
      ))}
    </div>
  )
}

// ---------- button ----------

export function Button(props: {
  children?: ReactNode
  onClick?: (e: React.MouseEvent) => void
  type?: 'default' | 'primary'
  status?: 'success' | 'danger' | 'warning'
  size?: 'small' | 'mini'
  disabled?: boolean
  loading?: boolean
  icon?: ReactNode
  long?: boolean
  className?: string
  title?: string
}) {
  const { children, onClick, type = 'default', status, size, disabled, loading, icon, long, className, title } = props
  const cls = [
    'btn',
    type === 'primary' ? 'btn-primary' : '',
    status ? `btn-${status}` : '',
    size ? `btn-${size}` : '',
    long ? 'btn-long' : '',
    loading ? 'is-loading' : '',
    className || '',
  ]
    .filter(Boolean)
    .join(' ')
  return (
    <button className={cls} disabled={disabled || loading} onClick={onClick} title={title}>
      {loading ? <Spinner size={14} /> : icon}
      {children}
    </button>
  )
}

export function Spinner({ size = 16 }: { size?: number }) {
  return <span className="spin" style={{ width: size, height: size }} />
}

// ---------- inputs ----------

export function Input(props: {
  value?: string
  onChange?: (v: string) => void
  placeholder?: string
  disabled?: boolean
  autoFocus?: boolean
  style?: React.CSSProperties
}) {
  return (
    <input
      className="inp"
      style={props.style}
      value={props.value}
      placeholder={props.placeholder}
      disabled={props.disabled}
      autoFocus={props.autoFocus}
      onChange={(e) => props.onChange?.(e.target.value)}
    />
  )
}

export function PasswordInput(props: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  autoFocus?: boolean
  onPressEnter?: () => void
}) {
  const [show, setShow] = useState(false)
  return (
    <div className="pwd">
      <input
        className="inp"
        type={show ? 'text' : 'password'}
        value={props.value}
        placeholder={props.placeholder}
        autoFocus={props.autoFocus}
        onChange={(e) => props.onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && props.onPressEnter) props.onPressEnter()
        }}
      />
      <button className="pwd-eye" tabIndex={-1} onClick={() => setShow(!show)}>
        {show ? <IconEyeOff size={15} /> : <IconEye size={15} />}
      </button>
    </div>
  )
}

// auto-growing textarea
export function TextArea(props: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  disabled?: boolean
  autoFocus?: boolean
  onKeyDown?: (e: React.KeyboardEvent) => void
}) {
  const ref = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 180) + 'px'
  }, [props.value])
  return (
    <textarea
      ref={ref}
      className="ta"
      rows={1}
      value={props.value}
      placeholder={props.placeholder}
      disabled={props.disabled}
      autoFocus={props.autoFocus}
      onChange={(e) => props.onChange(e.target.value)}
      onKeyDown={props.onKeyDown}
    />
  )
}

export function Select(props: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  placeholder?: string
  style?: React.CSSProperties
}) {
  return (
    <select
      className="sel"
      style={props.style}
      value={props.value}
      onChange={(e) => props.onChange(e.target.value)}
    >
      {props.placeholder !== undefined && <option value="">{props.placeholder}</option>}
      {props.options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  )
}

export function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button className={`tgl${checked ? ' on' : ''}`} onClick={() => onChange(!checked)}>
      <span className="tgl-dot" />
    </button>
  )
}

export function Tag({ children, color }: { children: ReactNode; color?: 'green' | 'red' | 'orange' | 'blue' }) {
  return <span className={`tag${color ? ' tag-' + color : ''}`}>{children}</span>
}

// ---------- confirm modal ----------

export function Confirm(props: {
  open: boolean
  title: string
  content?: ReactNode
  okText?: string
  danger?: boolean
  onOk: () => void
  onCancel: () => void
}) {
  if (!props.open) return null
  return (
    <div className="modal-mask" onClick={props.onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-title">{props.title}</div>
        {props.content && <div className="modal-body">{props.content}</div>}
        <div className="modal-actions">
          <Button onClick={props.onCancel}>取消</Button>
          <Button type="primary" status={props.danger ? 'danger' : undefined} onClick={props.onOk}>
            {props.okText || '确认'}
          </Button>
        </div>
      </div>
    </div>
  )
}

export function Empty({ description }: { description: string }) {
  return (
    <div className="empty">
      <svg width="42" height="42" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.4} strokeLinecap="round" strokeLinejoin="round">
        <rect x="3" y="7" width="18" height="13" rx="2" />
        <path d="M8 7V5a2 2 0 012-2h4a2 2 0 012 2v2" />
      </svg>
      <p>{description}</p>
    </div>
  )
}
