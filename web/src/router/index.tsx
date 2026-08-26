import { Navigate, Route, Routes } from 'react-router-dom'
import { AppLayout } from '../layouts/AppLayout'
import { LoginPage } from '../pages/login/LoginPage'
import { ProtectedRoute } from './ProtectedRoute'
import { appRoutes, type AppRouteDefinition } from './routes'

function renderRoute(route: AppRouteDefinition) {
  return (
    <Route key={route.path} path={route.path} element={route.element}>
      {route.indexRedirect ? (
        <Route index element={<Navigate to={route.indexRedirect} replace />} />
      ) : null}
      {route.children?.map(renderRoute)}
    </Route>
  )
}

export function RouterView() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <AppLayout />
          </ProtectedRoute>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        {appRoutes.map(renderRoute)}
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
