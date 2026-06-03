import { createContext } from 'react'

export type User = {
  id: number
  name: string
  email: string
  activated: boolean
  created_at: string
}

export type AuthContextType = {
  user: User | null
  token: string | null
  isAnonymous: boolean
  isLoading: boolean
  login: (token: string, user: User) => void
  enableAnonymous: () => void
  logout: () => void
}

export const AuthContext = createContext<AuthContextType | null>(null)
