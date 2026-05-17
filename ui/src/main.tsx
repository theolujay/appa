import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRouter, createRoute, createRootRouteWithContext, redirect } from '@tanstack/react-router'
import './index.css'
import { Dashboard } from './Dashboard'
import { Login } from './Login'
import { Register } from './Register'
import { Activate } from './Activate'
import { ToastProvider } from './Toast'
import { AuthProvider, useAuth } from './AuthContext'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
})

interface MyRouterContext {
  queryClient: QueryClient
  auth: ReturnType<typeof useAuth>
}

const rootRoute = createRootRouteWithContext<MyRouterContext>()()

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: ({ context }) => {
    if (!context.auth.token) {
      throw redirect({ to: '/login' })
    }
    if (context.auth.user && !context.auth.user.activated) {
      throw redirect({ to: '/activate' })
    }
  },
  component: Dashboard,
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  beforeLoad: ({ context }) => {
    if (context.auth.token) {
      throw redirect({ to: '/' })
    }
  },
  component: Login,
})

const registerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/register',
  component: Register,
})

const activateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/activate',
  component: Activate,
})

const routeTree = rootRoute.addChildren([indexRoute, loginRoute, registerRoute, activateRoute])

const router = createRouter({
  routeTree,
  context: {
    queryClient,
    auth: undefined!, // This will be set in the component
  },
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

function App() {
  const auth = useAuth()
  return <RouterProvider router={router} context={{ auth, queryClient }} />
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <AuthProvider>
          <App />
        </AuthProvider>
      </ToastProvider>
    </QueryClientProvider>
  </React.StrictMode>,
)
