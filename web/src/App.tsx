import { useEffect } from 'react'
import { Layout, Spin } from '@arco-design/web-react'
import Sidebar from './Sidebar'
import Chat from './pages/Chat'
import Login from './pages/Login'
import Settings from './pages/Settings'
import { useApp } from './store'

export default function App() {
  const { authed, bootstrap, view } = useApp()

  useEffect(() => {
    bootstrap()
  }, [bootstrap])

  if (authed === null) {
    return (
      <div className="boot">
        <Spin size={32} />
      </div>
    )
  }
  if (!authed) return <Login />

  return (
    <Layout className="app">
      <Sidebar />
      <Layout className="content">{view === 'settings' ? <Settings /> : <Chat />}</Layout>
    </Layout>
  )
}
