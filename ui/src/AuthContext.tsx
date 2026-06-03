import { useState, type ReactNode } from 'react'
import { AuthContext, type User } from './auth'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))
  const [isAnonymous, setIsAnonymous] = useState<boolean>(() => localStorage.getItem('isAnonymous') === 'true')
  const [user, setUser] = useState<User | null>(() => {
    const savedToken = localStorage.getItem('token')
    const savedUser = localStorage.getItem('user')
    try {
      return savedToken && savedUser ? JSON.parse(savedUser) : null
    } catch {
      return null
    }
  })
  const isLoading = false

  const login = (newToken: string, newUser: User) => {
    localStorage.setItem('token', newToken)
    localStorage.setItem('user', JSON.stringify(newUser))
    localStorage.removeItem('isAnonymous')
    setToken(newToken)
    setUser(newUser)
    setIsAnonymous(false)
  }

  const enableAnonymous = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('user')
    localStorage.setItem('isAnonymous', 'true')
    setToken(null)
    setUser(null)
    setIsAnonymous(true)
  }

  const logout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('user')
    localStorage.removeItem('isAnonymous')
    setToken(null)
    setUser(null)
    setIsAnonymous(false)
  }

  return (
    <AuthContext.Provider value={{ user, token, isAnonymous, login, enableAnonymous, logout, isLoading }}>
      {children}
    </AuthContext.Provider>
  )
}
