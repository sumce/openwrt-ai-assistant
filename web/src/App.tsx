import { useEffect } from 'react'
import Sidebar from './Sidebar'
import Chat from './pages/Chat'
import Login from './pages/Login'
import Settings from './pages/Settings'
import { ToastHost } from './ui'
import { useApp } from './store'

export default function App() {
  const { authed, bootstrap, view } = useApp()

  useEffect(() => {
    bootstrap()
  }, [bootstrap])

  if (authed === null) {
    return (
      <div className="boot">
        <span className="spin" style={{ width: 32, height: 32 }} />
      </div>
    )
  }
  if (!authed) return <Login />

  return (
    <div className="app">
      <Sidebar />
      <div className="content">{view === 'settings' ? <Settings /> : <Chat />}</div>
      <ToastHost />
    </div>
  )
}
