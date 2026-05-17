import { RouterProvider } from '@tanstack/react-router'
import { QueryClient } from '@tanstack/react-query'
import { useAuth } from './AuthContext'
import { router } from './router'

interface AppProps {
  queryClient: QueryClient
}

export function App({ queryClient }: AppProps) {
  const auth = useAuth()
  
  if (auth.isLoading) {
    return (
      <div className="loading-screen">
        <div className="spinner" />
      </div>
    )
  }

  return <RouterProvider router={router} context={{ auth, queryClient }} />
}
