import { createRouter, createRoute, createRootRouteWithContext, redirect } from '@tanstack/react-router'
import { QueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { Dashboard } from './Dashboard'
import { Login } from './Login'
import { Register } from './Register'
import { Activate } from './Activate'
import { useAuth } from './AuthContext'

export interface MyRouterContext {
  queryClient: QueryClient
  auth: ReturnType<typeof useAuth>
}

const rootRoute = createRootRouteWithContext<MyRouterContext>()()

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: ({ context }) => {
    if (!context.auth) return
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
    if (context.auth?.token) {
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

const activateSearchSchema = z.object({
  token: z.string().optional(),
})

const activateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/activate',
  validateSearch: (search) => activateSearchSchema.parse(search),
  component: Activate,
})

const routeTree = rootRoute.addChildren([indexRoute, loginRoute, registerRoute, activateRoute])

export const router = createRouter({
  routeTree,
  context: {
    queryClient: undefined!,
    auth: undefined!, 
  },
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
