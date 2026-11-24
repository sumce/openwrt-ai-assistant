import { useEffect, useState } from 'react'
import {
  Alert, Button, Card, Input, Message, Popconfirm, Select, Switch, Table, Tabs, Tag,
} from '@arco-design/web-react'
import { IconDelete, IconPlus, IconRefresh } from '@arco-design/web-react/icon'
import { api } from '../api'
import { useApp } from '../store'
import type { BackupInfo, Provider, Settings } from '../types'

export default function Settings() {
  const [st, setSt] = useState<Settings | null>(null)
  const [saving, setSaving] = useState(false)
  const [testingId, setTestingId] = useState('')
  const [modelOptions, setModelOptions] = useState<Record<string, string[]>>({})
  const [backups, setBackups] = useState<BackupInfo[]>([])
  const [tab, setTab] = useState('providers')

  useEffect(() => {
    api.getSettings().then((s) => setSt({ ...s, providers: s.providers || [] }))
  }, [])

  useEffect(() => {
    if (tab === 'backups') {
      api.backups().then(setBackups).catch(() => setBackups([]))
    }
  }, [tab])

  if (!st) return <div className="settings-loading">加载中…</div>

  const providers = st.providers || []

  const upd = (patch: Partial<Settings>) => setSt({ ...st, ...patch })

  const updProvider = (id: string, patch: Partial<Provider>) =>
    setSt({
      ...st,
      providers: providers.map((p) => (p.id === id ? { ...p, ...patch } : p)),
    })

  const addProvider = () =>
    setSt({
      ...st,
      providers: [
        ...providers,
        { id: '', name: '', base_url: '', api_key: '', model: '' } as Provider,
      ],
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
      const res = await api.testProvider({
        id: p.id,
        base_url: p.base_url,
        api_key: p.api_key,
      })
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

  const restore = async (name: string) => {
    try {
      const res = await api.restoreBackup(name)
      Message.success(res.message || '已恢复')
    } catch (e: any) {
      Message.error(e.message || '恢复失败')
    }
  }

  const changePassword = async (current: string, next: string, next2: string) => {
    if (next.length < 6) {
      Message.warning('新密码至少 6 位')
      return
    }
    if (next !== next2) {
      Message.warning('两次输入的新密码不一致')
      return
    }
    try {
      await api.changePassword(current, next)
      Message.success('密码已修改')
    } catch (e: any) {
      Message.error(e.message || '修改失败')
    }
  }

  return (
    <div className="settings">
      <div className="settings-header">
        <h2>设置</h2>
        <Button type="primary" loading={saving} onClick={save}>
          保存
        </Button>
      </div>

      <Tabs activeTab={tab} onChange={setTab} size="large">
        <Tabs.TabPane key="providers" title="模型服务">
          <Alert
            type="info"
            style={{ marginBottom: 16 }}
            content={
              <>
                配置任意 OpenAI 兼容上游（OpenAI / DeepSeek / 通义 / Kimi / Ollama / 各类中转站）。
                API 地址填到版本根路径，例如 <code>https://api.deepseek.com/v1</code>。
              </>
            }
          />

          <div className="field-row">
            <span className="field-label">默认模型服务</span>
            <Select
              style={{ width: 280 }}
              value={st.default_provider || ''}
              onChange={(v) => upd({ default_provider: v })}
              placeholder="选择默认服务"
            >
              {providers
                .filter((p) => p.id)
                .map((p) => (
                  <Select.Option key={p.id} value={p.id}>
                    {p.name || p.id}
                  </Select.Option>
                ))}
            </Select>
          </div>

          {providers.map((p, i) => {
            const key = p.id || `new-${i}`
            const opts = modelOptions[p.id] || []
            return (
              <Card
                key={key}
                className="provider-card"
                title={
                  <div className="provider-title">
                    <Input
                      style={{ width: 200 }}
                      placeholder="服务名称"
                      value={p.name}
                      onChange={(v) => updProvider(p.id || key, { name: v } as any)}
                    />
                    {st.default_provider === p.id && <Tag color="green">默认</Tag>}
                  </div>
                }
                extra={
                  <div className="provider-actions">
                    <Button
                      size="small"
                      icon={<IconRefresh />}
                      loading={testingId === key}
                      onClick={() => test(p)}
                      disabled={!p.base_url}
                    >
                      测试
                    </Button>
                    <Popconfirm title="删除这个模型服务？" onOk={() => delProvider(p.id)}>
                      <Button size="small" status="danger" icon={<IconDelete />} />
                    </Popconfirm>
                  </div>
                }
              >
                <div className="provider-grid">
                  <label>API 地址</label>
                  <Input
                    placeholder="https://api.deepseek.com/v1"
                    value={p.base_url}
                    onChange={(v) => {
                      const id = p.id || `tmp-${i}`
                      if (!p.id) {
                        setSt({
                          ...st,
                          providers: providers.map((x, j) =>
                            j === i ? { ...x, base_url: v } : x,
                          ),
                        })
                      } else {
                        updProvider(p.id, { base_url: v })
                      }
                    }}
                  />
                  <label>API Key</label>
                  <Input.Password
                    placeholder="sk-…"
                    value={p.api_key}
                    onChange={(v) => {
                      if (!p.id) {
                        setSt({
                          ...st,
                          providers: providers.map((x, j) => (j === i ? { ...x, api_key: v } : x)),
                        })
                      } else {
                        updProvider(p.id, { api_key: v })
                      }
                    }}
                  />
                  <label>模型</label>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <Input
                      placeholder="deepseek-chat"
                      value={p.model}
                      onChange={(v) => {
                        if (!p.id) {
                          setSt({
                            ...st,
                            providers: providers.map((x, j) => (j === i ? { ...x, model: v } : x)),
                          })
                        } else {
                          updProvider(p.id, { model: v })
                        }
                      }}
                    />
                    {opts.length > 0 && (
                      <Select
                        style={{ width: 220 }}
                        placeholder="从接口获取"
                        value=""
                        onChange={(v) => {
                          if (p.id) updProvider(p.id, { model: v })
                        }}
                      >
                        {opts.map((m) => (
                          <Select.Option key={m} value={m}>
                            {m}
                          </Select.Option>
                        ))}
                      </Select>
                    )}
                  </div>
                </div>
              </Card>
            )
          })}

          <Button long icon={<IconPlus />} onClick={addProvider}>
            添加模型服务
          </Button>
        </Tabs.TabPane>

        <Tabs.TabPane key="security" title="安全">
          <div className="security-row">
            <div>
              <div className="security-title">写操作需要确认</div>
              <div className="security-desc">
                修改配置（set_config、服务启停）时需要你在聊天界面点击批准。关闭后写操作将直接执行。
              </div>
            </div>
            <Switch checked={st.confirm_write} onChange={(v) => upd({ confirm_write: v })} />
          </div>
          <Alert
            type="warning"
            style={{ marginTop: 16 }}
            content={
              <>
                防火墙 / 网络 / 无线 / DHCP 属于危险操作，<b>任何时候</b>都需要确认，且执行前会自动备份当前配置（可在“备份”页回滚）。此项不可关闭。
              </>
            }
          />
        </Tabs.TabPane>

        <Tabs.TabPane key="backups" title={`备份${backups.length ? ` (${backups.length})` : ''}`}>
          <Alert
            type="info"
            style={{ marginBottom: 16 }}
            content={
              <>
                危险操作执行前自动生成的配置快照。恢复后如需生效，可能还需要重启对应服务（如
                <code>/etc/init.d/network reload</code>、<code>fw4 reload</code>）。
              </>
            }
          />
          <Table
            size="small"
            data={backups}
            pagination={{ pageSize: 10 }}
            noDataElement="暂无备份"
            columns={[
              {
                title: '时间',
                dataIndex: 'time',
                render: (v) => new Date(v * 1000).toLocaleString(),
              },
              {
                title: '配置',
                dataIndex: 'config',
                render: (v) => <Tag size="small">{v}</Tag>,
              },
              {
                title: '大小',
                dataIndex: 'size',
                render: (v) => `${(v / 1024).toFixed(1)} KB`,
              },
              {
                title: '操作',
                dataIndex: 'name',
                render: (_, row) => (
                  <Popconfirm title={`确认恢复备份 ${row.name}？`} onOk={() => restore(row.name)}>
                    <Button size="mini" status="warning">
                      恢复
                    </Button>
                  </Popconfirm>
                ),
              },
            ]}
          />
        </Tabs.TabPane>

        <Tabs.TabPane key="account" title="账户">
          <Card title="修改密码" className="provider-card">
            <PasswordForm onSubmit={changePassword} />
          </Card>
          <Card title="服务端口" className="provider-card" style={{ marginTop: 16 }}>
            <div className="field-row">
              <span className="field-label">当前端口</span>
              <Tag>{st.port}</Tag>
            </div>
            <Alert
              type="info"
              style={{ marginTop: 8 }}
              content="启用开关和端口在 LuCI → 服务 → WRT AI 中管理，修改后服务会自动重启。"
            />
          </Card>
        </Tabs.TabPane>
      </Tabs>
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
      <Input.Password value={cur} onChange={setCur} />
      <label>新密码</label>
      <Input.Password value={next} onChange={setNext} />
      <label>确认新密码</label>
      <Input.Password value={next2} onChange={setNext2} />
      <div />
      <Button type="primary" onClick={() => onSubmit(cur, next, next2)}>
        修改密码
      </Button>
    </div>
  )
}
