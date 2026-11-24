import { useEffect, useState } from 'react'
import { Button, Input, Message } from '@arco-design/web-react'
import { api } from '../api'
import { useApp } from '../store'

export default function Login() {
  const { initialized, setAuthed, loadConversations } = useApp()
  const [mode, setMode] = useState<'loading' | 'setup' | 'login'>('loading')
  const [pw, setPw] = useState('')
  const [pw2, setPw2] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (initialized !== null) {
      setMode(initialized ? 'login' : 'setup')
      return
    }
    api
      .bootstrap()
      .then((b: any) => setMode(b.initialized ? 'login' : 'setup'))
      .catch(() => setMode('login'))
  }, [initialized])

  const submit = async () => {
    if (mode === 'setup') {
      if (pw.length < 6) {
        Message.warning('密码至少 6 位')
        return
      }
      if (pw !== pw2) {
        Message.warning('两次输入的密码不一致')
        return
      }
    }
    setLoading(true)
    try {
      if (mode === 'setup') {
        await api.setup(pw)
        setMode('login')
      } else {
        await api.login(pw)
      }
      setAuthed(true)
      loadConversations()
    } catch (e: any) {
      Message.error(e.message || '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-brand">
          <span className="brand-logo big">⚡</span>
          <h1>WRT AI</h1>
          <p>运行在 OpenWRT 路由器上的 AI 助手</p>
        </div>

        {mode === 'loading' ? (
          <div className="login-hint">加载中…</div>
        ) : mode === 'setup' ? (
          <>
            <div className="login-hint">首次使用，请设置管理密码</div>
            <Input.Password
              placeholder="管理密码（至少 6 位）"
              value={pw}
              onChange={setPw}
              onPressEnter={submit}
            />
            <Input.Password
              placeholder="确认密码"
              value={pw2}
              onChange={setPw2}
              onPressEnter={submit}
            />
            <Button type="primary" long loading={loading} onClick={submit}>
              开始使用
            </Button>
          </>
        ) : (
          <>
            <Input.Password
              placeholder="管理密码"
              value={pw}
              onChange={setPw}
              onPressEnter={submit}
              autoFocus
            />
            <Button type="primary" long loading={loading} onClick={submit}>
              登录
            </Button>
          </>
        )}
      </div>
    </div>
  )
}
