import { useEffect, useState } from 'react'
import { api } from '../api'
import { useApp } from '../store'
import {
  Button, Confirm, IconPlus, IconRefresh, IconTrash, Input, Message, PasswordInput,
  Select, Spinner, Tag, Toggle,
} from '../ui'
import type { BackupInfo, Provider, Settings as SettingsT } from '../types'

const TABS = [
  { key: 'providers', label: '模型服务' },
  { key: 'security', label: '安全' },
  { key: 'backups', label: '备份' },
  { key: 'account', label: '账户' },
]

export default function Settings() {
  const [st, setSt] = useState<SettingsT | null>(null)
  const [saving, setSaving] = useState(false)
  const [testingId, setTestingId] = useState('')
  const [modelOptions, setModelOptions] = useState<Record<string, string[]>>({})
  const [backups, setBackups] = useState<BackupInfo[]>([])
  const [tab, setTab] = useState('providers')
  const [restoreName, setRestoreName] = useState<string | null>(null)

  useEffect(() => {
    api.getSettings().then((s) => setSt({ ...s, providers: s.providers || [] }))
  }, [])

  useEffect(() => {
    if (tab === 'backups') {
      api.backups().then(setBackups).catch(() => setBackups([]))
    }
  }, [tab])

  if (!st) {
    return (
      <div className="settings-loading">
        <Spinner size={22} />
      </div>
    )
  }

  const providers = st.providers || []

  const upd = (patch: Partial<SettingsT>) => setSt({ ...st, ...patch })

  const updProvider = (id: string, patch: Partial<Provider>) =>
    setSt({
      ...st,
      providers: providers.map((p) => (p.id === id ? { ...p, ...patch } : p)),
    })

  const updProviderByIndex = (i: number, patch: Partial<Provider>) =>
    setSt({
      ...st,
      providers: providers.map((p, j) => (j === i ? { ...p, ...patch } : p)),
    })

  const addProvider = () =>
    setSt({
      ...st,
      providers: [...providers, { id: '', name: '', base_url: '', api_key: '', model: '' }],
    })

  const delProvider = (id: string) => {
    const rest = providers.filter((p) => p.id !== id)
    setSt({
      ...st,
      providers: rest,
      default_provider: st.default_provider === id ? '' : st.default_provider,
    })
  }

  const save = async () => {
    setSaving(true)
    try {
      const saved = await api.saveSettings(st)
      setSt({ ...saved, providers: saved.providers || [] })
      Message.success('已保存')
    } catch (e: any) {
      Message.error(e.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const test = async (p: Provider) => {
    setTestingId(p.id || p.base_url)
    try {
      const res = await api.testProvider({ id: p.id, base_url: p.base_url, api_key: p.api_key })
      if (res.ok) {
        if (res.models && res.models.length) {
          setModelOptions((m) => ({ ...m, [p.id]: res.models }))
          Message.success(`连接成功，可用模型 ${res.models.length} 个`)
        } else {
          Message.success('连接成功')
        }
      } else {
        Message.error(res.error || '连接失败')
      }
    } catch (e: any) {
      Message.error(e.message || '连接失败')
    } finally {
      setTestingId('')
    }
  }

  const restore = async () => {
    if (!restoreName) return
    try {
      const res = await api.restoreBackup(restoreName)
      Message.success(res.message || '已恢复')
    } catch (e: any) {
      Message.error(e.message || '恢复失败')
    } finally {
      setRestoreName(null)
    }
  }

  const backupsTab = (
    <>
      <div className="alert alert-info">
        危险操作执行前自动生成的配置快照。恢复后如需生效，可能还需要重启对应服务（如
        <code>/etc/init.d/network reload</code>、<code>fw4 reload</code>）。
      </div>
      <table className="tbl">
        <thead>
          <tr>
            <th>时间</th>
            <th>配置</th>
            <th>大小</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {backups.length === 0 ? (
            <tr>
              <td colSpan={4} className="tbl-empty">暂无备份</td>
            </tr>
          ) : (
            backups.map((b) => (
              <tr key={b.name}>
                <td>{new Date(b.time * 1000).toLocaleString()}</td>
                <td><Tag>{b.config}</Tag></td>
                <td>{(b.size / 1024).toFixed(1)} KB</td>
                <td>
                  <Button size="mini" status="warning" onClick={() => setRestoreName(b.name)}>
                    恢复
                  </Button>
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </>
  )

  const securityTab = (
    <>
      <div className="security-row">
        <div>
          <div className="security-title">写操作需要确认</div>
          <div className="security-desc">
            修改配置（set_config、服务启停）时需要你在聊天界面点击批准。关闭后写操作将直接执行。
          </div>
        </div>
        <Toggle checked={st.confirm_write} onChange={(v) => upd({ confirm_write: v })} />
      </div>
      <div className="alert alert-warning" style={{ marginTop: 16 }}>
        防火墙 / 网络 / 无线 / DHCP 属于危险操作，<b>任何时候</b>都需要确认，且执行前会自动备份当前配置（可在“备份”页回滚）。此项不可关闭。
      </div>
    </>
  )

  const accountTab = (
    <>
      <div className="card">
        <div className="card-title">修改密码</div>
        <PasswordForm
          onSubmit={async (c, n, n2) => {
            if (n.length < 6) return Message.warning('新密码至少 6 位')
            if (n !== n2) return Message.warning('两次输入的新密码不一致')
            try {
              await api.changePassword(c, n)
              Message.success('密码已修改')
            } catch (e: any) {
              Message.error(e.message || '修改失败')
            }
          }}
        />
      </div>
      <div className="card" style={{ marginTop: 16 }}>
        <div className="card-title">服务端口</div>
        <div className="field-row">
          <span className="field-label">当前端口</span>
          <Tag>{st.port}</Tag>
        </div>
        <div className="alert alert-info">
          启用开关和端口在 LuCI → 服务 → WRT AI 中管理，修改后服务会自动重启。
        </div>
      </div>
    </>
  )

  return (
    <div className="settings">
      <div className="settings-header">
        <h2>设置</h2>
        <Button type="primary" loading={saving} onClick={save}>
          保存
        </Button>
      </div>

      <div className="tabs">
        {TABS.map((t) => (
          <button
            key={t.key}
            className={`tab${tab === t.key ? ' active' : ''}`}
            onClick={() => setTab(t.key)}
          >
            {t.label}
            {t.key === 'backups' && backups.length > 0 ? ` (${backups.length})` : ''}
          </button>
        ))}
      </div>

      {tab === 'providers' && (
        <div className="tabpane">
          <div className="alert alert-info">
            配置任意 OpenAI 兼容上游（OpenAI / DeepSeek / 通义 / Kimi / Ollama / 各类中转站）。
            API 地址填到版本根路径，例如 <code>https://api.deepseek.com/v1</code>。
          </div>

          <div className="field-row">
            <span className="field-label">默认模型服务</span>
            <Select
              style={{ width: 280 }}
              value={st.default_provider || ''}
              onChange={(v) => upd({ default_provider: v })}
              placeholder="选择默认服务"
              options={providers.filter((p) => p.id).map((p) => ({ value: p.id, label: p.name || p.id }))}
            />
          </div>

          {providers.map((p, i) => {
            const key = p.id || `new-${i}`
            const opts = modelOptions[p.id] || []
            return (
              <div key={key} className="card provider-card">
                <div className="card-head">
                  <div className="provider-title">
                    <Input
                      style={{ width: 200 }}
                      placeholder="服务名称"
                      value={p.name}
                      onChange={(v) => updProviderByIndex(i, { name: v })}
                    />
                    {st.default_provider === p.id && <Tag color="green">默认</Tag>}
                  </div>
                  <div className="provider-actions">
                    <Button
                      size="small"
                      icon={<IconRefresh size={14} />}
                      loading={testingId === key}
                      disabled={!p.base_url}
                      onClick={() => test(p)}
                    >
                      测试
                    </Button>
                    <Button size="small" status="danger" icon={<IconTrash size={14} />} onClick={() => delProvider(p.id)} title="删除" />
                  </div>
                </div>
                <div className="provider-grid">
                  <label>API 地址</label>
                  <Input
                    placeholder="https://api.deepseek.com/v1"
                    value={p.base_url}
                    onChange={(v) => updProviderByIndex(i, { base_url: v })}
                  />
                  <label>API Key</label>
                  <PasswordInput
                    placeholder="sk-…"
                    value={p.api_key}
                    onChange={(v) => updProviderByIndex(i, { api_key: v })}
                  />
                  <label>模型</label>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <Input
                      placeholder="deepseek-chat"
                      value={p.model}
                      onChange={(v) => updProviderByIndex(i, { model: v })}
                    />
                    {opts.length > 0 && (
                      <Select
                        style={{ width: 220 }}
                        placeholder="从接口获取"
                        value=""
                        onChange={(v) => p.id && updProvider(p.id, { model: v })}
                        options={opts.map((m) => ({ value: m, label: m }))}
                      />
                    )}
                  </div>
                </div>
              </div>
            )
          })}

          <Button long icon={<IconPlus size={15} />} onClick={addProvider}>
            添加模型服务
          </Button>
        </div>
      )}

      {tab === 'security' && securityTab}
      {tab === 'backups' && backupsTab}
      {tab === 'account' && accountTab}

      <Confirm
        open={!!restoreName}
        title={`确认恢复备份 ${restoreName || ''}？`}
        danger
        onOk={restore}
        onCancel={() => setRestoreName(null)}
      />
    </div>
  )
}

function PasswordForm({ onSubmit }: { onSubmit: (c: string, n: string, n2: string) => void }) {
  const [cur, setCur] = useState('')
  const [next, setNext] = useState('')
  const [next2, setNext2] = useState('')
  return (
    <div className="provider-grid">
      <label>当前密码</label>
      <PasswordInput value={cur} onChange={setCur} />
      <label>新密码</label>
      <PasswordInput value={next} onChange={setNext} />
      <label>确认新密码</label>
      <PasswordInput value={next2} onChange={setNext2} />
      <div />
      <Button type="primary" onClick={() => onSubmit(cur, next, next2)}>
        修改密码
      </Button>
    </div>
  )
}
