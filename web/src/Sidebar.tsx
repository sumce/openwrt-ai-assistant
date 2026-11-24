import { useEffect, useState } from 'react'
import { api } from './api'
import { useApp } from './store'
import {
  Button, Confirm, IconLogout, IconPanel, IconPlus, IconSettings, Tag,
} from './ui'

export default function Sidebar() {
  const {
    conversations, currentConvId, dev, version, view,
    setView, setCurrentConv, loadConversations, logout,
  } = useApp()

  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('wrtai_sidebar') === '0')
  const [delId, setDelId] = useState<string | null>(null)

  useEffect(() => {
    loadConversations()
  }, [loadConversations])

  const toggle = () => {
    setCollapsed((c) => {
      localStorage.setItem('wrtai_sidebar', c ? '1' : '0')
      return !c
    })
  }

  const newChat = () => {
    setCurrentConv(null)
    setView('chat')
  }

  const del = async () => {
    if (!delId) return
    try {
      await api.deleteConversation(delId)
      if (delId === currentConvId) setCurrentConv(null)
      setDelId(null)
      loadConversations()
    } catch (e) {
      console.error(e)
    }
  }

  if (collapsed) {
    return (
      <div className="sider sider-rail">
        <div className="brand-rail" title="新会话" onClick={newChat}>⚡</div>
        <div className="rail-body">
          <button className="rail-btn" title="新会话" onClick={newChat}>
            <IconPlus size={17} />
          </button>
        </div>
        <div className="rail-footer">
          <button
            className={`rail-btn${view === 'settings' ? ' active' : ''}`}
            title="设置"
            onClick={() => setView(view === 'settings' ? 'chat' : 'settings')}
          >
            <IconSettings size={17} />
          </button>
          <button className="rail-btn" title="展开侧栏" onClick={toggle}>
            <IconPanel size={17} />
          </button>
        </div>
        <Confirm open={!!delId} title="删除这个会话？" danger onOk={del} onCancel={() => setDelId(null)} />
      </div>
    )
  }

  return (
    <div className="sider">
      <div className="brand" onClick={newChat}>
        <span className="brand-logo">⚡</span>
        <span className="brand-name">WRT AI</span>
        {dev && <Tag color="orange">DEV</Tag>}
        <button className="sider-collapse" title="收起侧栏" onClick={toggle}>
          <IconPanel size={16} />
        </button>
      </div>

      <div className="sider-body">
        <Button type="primary" long icon={<IconPlus size={15} />} onClick={newChat}>
          新会话
        </Button>
        <div className="conv-list">
          {conversations.map((c) => (
            <div
              key={c.id}
              className={`conv-item${c.id === currentConvId && view === 'chat' ? ' active' : ''}`}
              onClick={() => { setCurrentConv(c.id); setView('chat') }}
            >
              <span className="conv-title">{c.title}</span>
              <span
                className="conv-del"
                title="删除会话"
                onClick={(e) => { e.stopPropagation(); setDelId(c.id) }}
              >
                ✕
              </span>
            </div>
          ))}
        </div>
      </div>

      <div className="sider-footer">
        <Button
          long
          size="small"
          icon={<IconSettings size={15} />}
          className={view === 'settings' ? 'settings-btn active' : 'settings-btn'}
          onClick={() => setView(view === 'settings' ? 'chat' : 'settings')}
        >
          设置
        </Button>
        <div className="sider-meta">
          <span>v{version}</span>
          <a onClick={logout}>退出登录</a>
        </div>
      </div>

      <Confirm open={!!delId} title="删除这个会话？" danger onOk={del} onCancel={() => setDelId(null)} />
    </div>
  )
}
