import { useEffect } from 'react'
import { Button, Layout, Popconfirm, Tag } from '@arco-design/web-react'
import { IconDelete, IconPlus, IconSettings } from '@arco-design/web-react/icon'
import { api } from './api'
import { useApp } from './store'

export default function Sidebar() {
  const {
    conversations, currentConvId, dev, version, view,
    setView, setCurrentConv, loadConversations, logout,
  } = useApp()

  useEffect(() => {
    loadConversations()
  }, [loadConversations])

  const newChat = () => {
    setCurrentConv(null)
    setView('chat')
  }

  const del = async (id: string) => {
    try {
      await api.deleteConversation(id)
      if (id === currentConvId) setCurrentConv(null)
      loadConversations()
    } catch (e) {
      console.error(e)
    }
  }

  return (
    <Layout.Sider className="sider" width={252}>
      <div className="brand" onClick={newChat}>
        <span className="brand-logo">⚡</span>
        <span className="brand-name">WRT AI</span>
        {dev && <Tag size="small" color="orange" style={{ marginLeft: 8 }}>DEV</Tag>}
      </div>

      <div className="sider-body">
        <Button type="primary" long icon={<IconPlus />} onClick={newChat}>
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
              <Popconfirm title="删除这个会话？" onOk={() => del(c.id)}>
                <IconDelete
                  className="conv-del"
                  onClick={(e) => e.stopPropagation()}
                />
              </Popconfirm>
            </div>
          ))}
        </div>
      </div>

      <div className="sider-footer">
        <Button
          long
          size="small"
          icon={<IconSettings />}
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
    </Layout.Sider>
  )
}
