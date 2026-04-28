import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import Ansi from 'ansi-to-react'
import { config } from './config'
import { useToast } from './useToast'

const AnsiComponent = (Ansi as any).default || Ansi;

interface Deployment {
  id: string
  source: string
  status: 'pending' | 'building' | 'deploying' | 'running' | 'failed' | 'canceled' | 'stopped'
  image_tag: string | null
  address: string | null
  env_vars: string | null
  url: string | null
  created_at: string
}

interface LogEntry {
  time: Date
  text: string
}

interface WSLogEvent {
  type: 'log'
  log: {
    id: number
    line: string
  }
}

interface WSStatusEvent {
  type: 'status'
  status: {
    status: Deployment['status']
    url?: string
  }
}

type WSEvent = WSLogEvent | WSStatusEvent

const API_BASE = config.apiUrl
const WS_BASE = config.wsUrl

export function Dashboard() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  // Initialize state from URL hash if present
  const [selectedId, setSelectedIdState] = useState<string | null>(() => {
    const hash = window.location.hash.replace('#', '')
    return hash || null
  })

  // Wrapper to keep URL in sync
  const setSelectedId = useCallback((id: string | null) => {
    setSelectedIdState(id)
    if (id) {
      window.location.hash = id
    } else {
      window.location.hash = ''
    }
  }, [])

  // Sync state if user navigates via browser back/forward
  useEffect(() => {
    const handleHashChange = () => {
      const hash = window.location.hash.replace('#', '')
      setSelectedIdState(hash || null)
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  const [gitUrl, setGitUrl] = useState('')
  const [envVars, setEnvVars] = useState('')
  const [isDragging, setIsDragging] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const { data: deployments, isLoading } = useQuery({
    queryKey: ['deployments'],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/deployments`)
      if (!res.ok) throw new Error('Failed to fetch deployments')
      return res.json() as Promise<Deployment[]>
    },
    refetchInterval: 10000,
  })

  const deployMutation = useMutation({
    mutationFn: async (input: { source: string; env_vars: string }) => {
      const res = await fetch(`${API_BASE}/deployments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) {
        const err = await res.text()
        throw new Error(err || 'Failed to trigger deployment')
      }
      return res.json()
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['deployments'] })
      setGitUrl('')
      setEnvVars('')
      setSelectedId(data.id)
      addToast('Deployment started', 'success')
    },
    onError: (err: Error) => {
      addToast(err.message, 'error')
    },
  })

  const uploadMutation = useMutation({
    mutationFn: async (input: { file: File; env_vars: string }) => {
      const formData = new FormData()
      formData.append('file', input.file)
      formData.append('env_vars', input.env_vars)
      const res = await fetch(`${API_BASE}/deployments/upload`, {
        method: 'POST',
        body: formData,
      })
      if (!res.ok) {
        const err = await res.text()
        throw new Error(err || 'Failed to upload project')
      }
      return res.json()
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['deployments'] })
      setEnvVars('')
      setSelectedId(data.id)
      addToast('Project uploaded and deploying', 'success')
    },
    onError: (err: Error) => {
      addToast(err.message, 'error')
    },
  })

  const cancelMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/deployments/${id}`, {
        method: 'PATCH',
      })
      if (!res.ok) {
        const err = await res.text()
        throw new Error(err || 'Failed to cancel deployment')
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployments'] })
      addToast('Action requested', 'info')
    },
    onError: (err: Error) => {
      addToast(err.message, 'error')
    },
  })

  const isSubmitting = deployMutation.isPending || uploadMutation.isPending

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!gitUrl.trim() || isSubmitting) return
    deployMutation.mutate({ source: gitUrl, env_vars: envVars })
  }

  const handleStatusUpdate = useCallback((id: string, status: Deployment['status'], url?: string) => {
    queryClient.setQueryData(['deployments'], (old: Deployment[] | undefined) => {
      if (!old) return old
      return old.map((d) => (d.id === id ? { ...d, status, url: url || d.url } : d))
    })
  }, [queryClient])

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(true)
  }, [])

  const onDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(false)
  }, [])

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(false)
    const files = e.dataTransfer.files
    if (files && files.length > 0) {
      const file = files[0]
      if (file.name.endsWith('.zip')) {
        uploadMutation.mutate({ file, env_vars: envVars })
      } else {
        addToast('Please upload a .zip file', 'error')
      }
    }
  }, [uploadMutation, envVars, addToast])

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files
    if (files && files.length > 0) {
      uploadMutation.mutate({ file: files[0], env_vars: envVars })
    }
  }

  const selectedDeployment = deployments?.find((d) => d.id === selectedId)

  return (
    <div className="app-container">
      <div className="left-panel">
        <div className="header">
          <h1>appa</h1>
        </div>

        <div
          className={`deploy-form ${isDragging ? 'dragging' : ''}`}
          onDragOver={onDragOver}
          onDragLeave={onDragLeave}
          onDrop={onDrop}
        >
          <form onSubmit={handleSubmit}>
            <div className="input-group">
              <label htmlFor="git-url">Git Repository URL</label>
              <input
                id="git-url"
                type="text"
                placeholder="https://github.com/user/repo"
                value={gitUrl}
                onChange={(e) => setGitUrl(e.target.value)}
                disabled={isSubmitting}
              />
            </div>

            <div className="input-group">
              <label htmlFor="env-vars">Environment Variables</label>
              <textarea
                id="env-vars"
                placeholder="KEY=VALUE&#10;PORT=8080"
                value={envVars}
                onChange={(e) => setEnvVars(e.target.value)}
                disabled={isSubmitting}
                className="env-textarea"
              />
            </div>

            <button
              type="submit"
              className="btn-primary"
              disabled={isSubmitting || !gitUrl.trim()}
            >
              {deployMutation.isPending ? <><span className="spinner" /> Deploying...</> : 'Deploy'}
            </button>
          </form>

          <div className="upload-divider">
            <span>OR</span>
          </div>

          <div className="file-upload-zone" onClick={() => fileInputRef.current?.click()}>
            <input
              type="file"
              ref={fileInputRef}
              style={{ display: 'none' }}
              accept=".zip"
              onChange={handleFileChange}
            />
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12"/>
            </svg>
            <p>{uploadMutation.isPending ? 'Uploading...' : 'Drop ZIP or click to upload'}</p>
          </div>
        </div>

        <div className="deployments-list" role="listbox" aria-label="Deployments">
          {isLoading && <div className="empty-state">Loading deployments...</div>}
          {!isLoading && deployments?.length === 0 && (
            <div className="empty-state">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
              </svg>
              <p>No deployments yet</p>
              <p className="hint">Deploy your first project above</p>
            </div>
          )}
          {deployments?.map((d) => (
            <div
              key={d.id}
              role="option"
              aria-selected={selectedId === d.id}
              tabIndex={0}
              className={`deployment-item ${selectedId === d.id ? 'active' : ''}`}
              onClick={() => setSelectedId(d.id)}
              onKeyDown={(e) => e.key === 'Enter' && setSelectedId(d.id)}
            >
              <div className="deployment-meta">
                <span className={`status-badge status-${d.status}`}>{d.status}</span>
                <span className="id-mono">{d.id.substring(0, 8)}</span>
              </div>
              <div className="source-url" title={d.source}>{d.source}</div>
              {d.url && d.status === 'running' && (
                <a
                  href={d.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="live-url"
                  onClick={(e) => e.stopPropagation()}
                >
                  Visit Site →
                </a>
              )}
            </div>
          ))}
        </div>
      </div>

      <div className="right-panel">
        {selectedDeployment ? (
          <>
            <div className="selected-header">
              <div className="top">
                <div className="title-row">
                  <h2>{selectedDeployment.id.substring(0, 8)}</h2>
                  <span className={`status-badge status-${selectedDeployment.status}`}>
                    {selectedDeployment.status}
                  </span>
                </div>
                <div className="actions">
                  {(['pending', 'building', 'deploying', 'running'].includes(selectedDeployment.status)) && (
                    <button
                      className="btn-danger btn-sm"
                      onClick={() => {
                        if (selectedDeployment.status === 'running') {
                          if (confirm('Are you sure you want to stop this application?')) {
                            cancelMutation.mutate(selectedDeployment.id)
                          }
                        } else {
                          cancelMutation.mutate(selectedDeployment.id)
                        }
                      }}
                      disabled={cancelMutation.isPending}
                    >
                      {cancelMutation.isPending ? 'Processing...' : selectedDeployment.status === 'running' ? 'Stop' : 'Cancel'}
                    </button>
                  )}
                </div>
              </div>
              <div className="meta-grid">
                <div className="meta-item">
                  <label>Source</label>
                  <span>{selectedDeployment.source}</span>
                </div>
                <div className="meta-item">
                  <label>Created At</label>
                  <span>{new Date(selectedDeployment.created_at).toLocaleString()}</span>
                </div>
                {selectedDeployment.address && (
                  <div className="meta-item">
                    <label>Internal Address</label>
                    <span className="id-mono">{selectedDeployment.address}</span>
                  </div>
                )}
                {selectedDeployment.image_tag && (
                  <div className="meta-item">
                    <label>Image Tag</label>
                    <span className="id-mono">{selectedDeployment.image_tag}</span>
                  </div>
                )}
                {selectedDeployment.env_vars && (
                   <div className="meta-item full-width">
                     <label>Configured Env Vars</label>
                     <pre className="env-display">{selectedDeployment.env_vars}</pre>
                   </div>
                )}
              </div>
            </div>
            <LogPanel
              key={selectedDeployment.id}
              deploymentId={selectedDeployment.id}
              onStatusUpdate={(status, url) => handleStatusUpdate(selectedDeployment.id, status, url)}
            />
          </>
        ) : (
          <div className="empty-state">
            <svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 8l4 4-4 4M8 12h8" />
            </svg>
            <p>Select a deployment to view logs</p>
          </div>
        )}
      </div>
    </div>
  )
}

function LogPanel({
  deploymentId,
  onStatusUpdate
}: {
  deploymentId: string;
  onStatusUpdate: (status: Deployment['status'], url?: string) => void
}) {
  const { addToast } = useToast()
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [wsStatus, setWsStatus] = useState<'connecting' | 'connected' | 'reconnecting'>('connecting')
  const [searchQuery, setSearchQuery] = useState('')
  const logEndRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeoutRef = useRef<number | undefined>(undefined)

  const filteredLogs = useMemo(() => {
    if (!searchQuery.trim()) return logs
    const q = searchQuery.toLowerCase()
    return logs.filter((l) => l.text.toLowerCase().includes(q))
  }, [logs, searchQuery])

  useEffect(() => {
    const connect = () => {
      if (wsRef.current) {
        wsRef.current.onclose = null
        wsRef.current.close()
      }

      setWsStatus('connecting')
      const ws = new WebSocket(`${WS_BASE}/deployments/${deploymentId}/logs`)
      wsRef.current = ws

      ws.onopen = () => setWsStatus('connected')

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as WSEvent
          if (data.type === 'log') {
            setLogs((prev) => [...prev, { time: new Date(), text: data.log.line }])
          } else if (data.type === 'status') {
            onStatusUpdate(data.status.status, data.status.url)
            addToast(`Deployment is now ${data.status.status}`, 'info')
          }
        } catch {
          setLogs((prev) => [...prev, { time: new Date(), text: event.data }])
        }
      }

      ws.onclose = () => {
        setWsStatus('reconnecting')
        // Short delay for quick recovery if Caddy reloads during handshake
        reconnectTimeoutRef.current = window.setTimeout(connect, 500)
      }

      ws.onerror = () => ws.close()
    }

    connect()
    return () => {
      if (wsRef.current) {
        wsRef.current.onclose = null
        wsRef.current.close()
      }
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current)
    }
  }, [deploymentId, onStatusUpdate, addToast])

  useEffect(() => {
    if (autoScroll && logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [filteredLogs, autoScroll])

  const handleScroll = () => {
    if (!containerRef.current) return
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50
    setAutoScroll(isAtBottom)
  }

  const copyLogs = () => {
    const text = logs.map((l) => `[${l.time.toLocaleTimeString()}] ${l.text}`).join('\n')
    navigator.clipboard.writeText(text)
    addToast('Logs copied to clipboard', 'success')
  }

  return (
    <div className="log-panel">
      <div className="log-toolbar">
        <div className="ws-indicator" data-status={wsStatus}>
          <span className="ws-dot" />
          {wsStatus === 'connected' ? 'Connected' : 'Reconnecting...'}
        </div>

        <div className="log-search-container">
          <input
            type="text"
            className="log-search-input"
            placeholder="Filter logs..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>

        <div className="log-controls">
          <button
            className="btn-icon"
            onClick={copyLogs}
            disabled={logs.length === 0}
            aria-label="Copy logs"
            title="Copy logs"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
              <rect x="8" y="2" width="8" height="4" rx="1" />
            </svg>
          </button>
          {!autoScroll && (
            <button
              className="btn-sm btn-primary"
              onClick={() => {
                setAutoScroll(true)
                logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
              }}
            >
              Resume Follow
            </button>
          )}
        </div>
      </div>
      <div
        className="log-container"
        ref={containerRef}
        onScroll={handleScroll}
        tabIndex={0}
        role="log"
      >
        {filteredLogs.length === 0 && wsStatus === 'connected' && (
          <div className="log-empty">{searchQuery ? 'No logs match your filter' : 'Waiting for logs...'}</div>
        )}
        {filteredLogs.map((log, i) => (
          <div key={i} className="log-line">
            <span className="log-time">{log.time.toLocaleTimeString()}</span>
            <span className="log-text">
              <AnsiComponent linkify>{log.text}</AnsiComponent>
            </span>
          </div>
        ))}
        <div ref={logEndRef} />
      </div>
    </div>
  )
}
