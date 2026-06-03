import { useState } from 'react'
import { useNavigate, useSearch, Link } from '@tanstack/react-router'
import { config } from './config'
import { useToast } from './useToast'
import { useAuth } from './useAuth'

const API_BASE = config.apiUrl

export function Activate() {
  const search = useSearch({ from: '/activate' }) as { token?: string }
  const [token, setToken] = useState(() => search.token ?? '')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const { logout } = useAuth()
  const { addToast } = useToast()
  const navigate = useNavigate()

  const handleSubmit = async (e?: React.SubmitEvent) => {
    e?.preventDefault()
    if (!token || isSubmitting) return

    setIsSubmitting(true)
    try {
      const res = await fetch(`${API_BASE}/v1/users/activated`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token }),
      })

      const data = await res.json()

      if (!res.ok) {
        throw new Error(data.error || 'Activation failed')
      }

      logout()
      addToast('Account activated! You can now log in.', 'success')
      navigate({ to: '/login' })
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Activation failed'
      addToast(message, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="auth-container">
      <div className="auth-card">
        <h1>Activate Account</h1>
        <p className="auth-subtitle">Enter your activation token to continue</p>

        <form onSubmit={handleSubmit}>
          <div className="input-group">
            <label htmlFor="token">Activation Token</label>
            <input
              id="token"
              type="text"
              placeholder="YOUR_TOKEN"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              required
            />
          </div>
          <button type="submit" className="btn-primary" disabled={isSubmitting}>
            {isSubmitting ? <span className="spinner" /> : 'Activate'}
          </button>
        </form>

        <p className="auth-footer">
          Wait, I remember my password. <Link to="/login">Back to Login</Link>
        </p>
      </div>
    </div>
  )
}
