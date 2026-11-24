import { create } from 'zustand'
import { api } from './api'
import type { Conversation } from './types'

interface AppState {
  authed: boolean | null // null = bootstrapping
  initialized: boolean | null // admin password set up?
  dev: boolean
  version: string
  view: 'chat' | 'settings'
  conversations: Conversation[]
  currentConvId: string | null

  bootstrap: () => Promise<void>
  setAuthed: (v: boolean) => void
  setView: (v: 'chat' | 'settings') => void
  loadConversations: () => Promise<void>
  setCurrentConv: (id: string | null) => void
  logout: () => Promise<void>
}

export const useApp = create<AppState>((set) => ({
  authed: null,
  initialized: null,
  dev: false,
  version: '',
  view: 'chat',
  conversations: [],
  currentConvId: null,

  bootstrap: async () => {
    try {
      const b = await api.bootstrap()
      set({ dev: !!b.dev, version: b.version, initialized: !!b.initialized })
      if (!b.initialized) {
        set({ authed: false })
        return
      }
      // Probe the session with a cheap authed endpoint (cookie may still
      // be valid from a previous visit).
      try {
        await api.activeRuns()
        set({ authed: true })
      } catch {
        set({ authed: false })
      }
    } catch {
      set({ authed: false, initialized: false })
    }
  },

  setAuthed: (v) => set({ authed: v }),
  setView: (v) => set({ view: v }),

  loadConversations: async () => {
    try {
      set({ conversations: await api.conversations() })
    } catch {
      /* ignore */
    }
  },

  setCurrentConv: (id) => set({ currentConvId: id }),

  logout: async () => {
    try {
      await api.logout()
    } catch {
      /* ignore */
    }
    set({ authed: false, view: 'chat', conversations: [], currentConvId: null })
  },
}))
