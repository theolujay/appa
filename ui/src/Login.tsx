import { useState } from 'react'
import { useNavigate, Link } from '@tanstack/react-router'
import { config } from './config'
import { useToast } from './useToast'
import { useAuth } from './useAuth'

const API_BASE = config.apiUrl

export function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const { addToast } = useToast()
  const { login, enableAnonymous } = useAuth()
  const navigate = useNavigate()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!email || !password || isSubmitting) return

    setIsSubmitting(true)
    try {
      const res = await fetch(`${API_BASE}/v1/tokens/authentication`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      })

      const data = await res.json()

      if (!res.ok) {
        throw new Error(data.error || 'Invalid credentials')
      }

      login(data.authentication_token.token, data.user)
      addToast('Logged in successfully', 'success')
      navigate({ to: '/' })
    } catch (err: any) {
      addToast(err.message, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleContinueAsGuest = () => {
    enableAnonymous()
    addToast('Continuing as guest', 'info')
    navigate({ to: '/' })
  }

  return (
    <div className="auth-container">
      <div className="auth-card">
        <h1>Welcome Back</h1>
        <p className="auth-subtitle">Login to your appa account</p>
        
        <form onSubmit={handleSubmit}>
          <div className="input-group">
            <label htmlFor="email">Email Address</label>
            <input
              id="email"
              type="email"
              placeholder="you@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
          <div className="input-group">
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              placeholder="••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          <button type="submit" className="btn-primary" disabled={isSubmitting}>
            {isSubmitting ? <span className="spinner" /> : 'Login'}
          </button>
        </form>

        <div className="auth-divider">
          <span>OR</span>
        </div>

        <button className="btn-ghost" onClick={handleContinueAsGuest}>
          Continue as Guest
        </button>

        <p className="auth-footer">
          Don't have an account? <Link to="/register">Register</Link>
        </p>
      </div>
    </div>
  )
}
