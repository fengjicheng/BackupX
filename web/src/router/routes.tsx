import type { ReactElement, ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import {
  IconBook,
  IconCopy,
  IconDashboard,
  IconDesktop,
  IconFile,
  IconFilePdf,
  IconHistory,
  IconList,
  IconNotification,
  IconRefresh,
  IconSafe,
  IconSettings,
  IconStorage,
  IconUser,
} from '../components/icons'
import { AdminLayout } from '../pages/admin/AdminLayout'
import { ApiKeysPage } from '../pages/admin/ApiKeysPage'
import { UsersPage } from '../pages/admin/UsersPage'
import { AuditLogsPage } from '../pages/audit/AuditLogsPage'
import { BackupRecordsPage } from '../pages/backup-records/BackupRecordsPage'
import { BackupTasksPage } from '../pages/backup-tasks/BackupTasksPage'
import { DashboardPage } from '../pages/dashboard/DashboardPage'
import NodesPage from '../pages/nodes/NodesPage'
import { NotificationsPage } from '../pages/notifications/NotificationsPage'
import { ReplicationRecordsPage } from '../pages/replication-records/ReplicationRecordsPage'
import { ReportsPage } from '../pages/reports/ReportsPage'
import { RestoreRecordsPage } from '../pages/restore-records/RestoreRecordsPage'
import { SettingsPage } from '../pages/settings/SettingsPage'
import { GoogleDriveCallbackPage } from '../pages/storage-targets/GoogleDriveCallbackPage'
import { StorageTargetsPage } from '../pages/storage-targets/StorageTargetsPage'
import { TaskTemplatesPage } from '../pages/task-templates/TaskTemplatesPage'
import { VerificationRecordsPage } from '../pages/verification-records/VerificationRecordsPage'

interface MenuMetadata {
  label: string
  icon: ReactNode
  adminOnly?: boolean
  matchAliases?: readonly string[]
}

export interface AppRouteDefinition {
  path: string
  element: ReactElement
  indexRedirect?: string
  children?: readonly AppRouteDefinition[]
  menu?: MenuMetadata
}

export const appRoutes: readonly AppRouteDefinition[] = [
  {
    path: 'dashboard',
    element: <DashboardPage />,
    menu: { label: '仪表盘', icon: <IconDashboard /> },
  },
  {
    path: 'reports',
    element: <ReportsPage />,
    menu: { label: '合规报表', icon: <IconFilePdf /> },
  },
  {
    path: 'backup/tasks',
    element: <BackupTasksPage />,
    menu: { label: '备份任务', icon: <IconFile /> },
  },
  {
    path: 'backup/records',
    element: <BackupRecordsPage />,
    menu: { label: '备份记录', icon: <IconHistory /> },
  },
  {
    path: 'restore/records',
    element: <RestoreRecordsPage />,
    menu: { label: '恢复记录', icon: <IconRefresh /> },
  },
  {
    path: 'verify/records',
    element: <VerificationRecordsPage />,
    menu: { label: '验证演练', icon: <IconSafe /> },
  },
  {
    path: 'replication/records',
    element: <ReplicationRecordsPage />,
    menu: { label: '备份复制', icon: <IconCopy /> },
  },
  {
    path: 'task-templates',
    element: <TaskTemplatesPage />,
    menu: { label: '任务模板', icon: <IconBook /> },
  },
  {
    path: 'storage-targets',
    element: <StorageTargetsPage />,
    menu: { label: '存储目标', icon: <IconStorage /> },
  },
  {
    path: 'storage-targets/google-drive/callback',
    element: <GoogleDriveCallbackPage />,
  },
  {
    path: 'nodes',
    element: <NodesPage />,
    menu: { label: '节点管理', icon: <IconDesktop /> },
  },
  {
    path: 'settings/notifications',
    element: <NotificationsPage />,
    menu: { label: '通知配置', icon: <IconNotification /> },
  },
  {
    path: 'admin',
    element: <AdminLayout />,
    indexRedirect: 'users',
    menu: { label: '访问管理', icon: <IconUser />, adminOnly: true },
    children: [
      { path: 'users', element: <UsersPage /> },
      { path: 'api-keys', element: <ApiKeysPage /> },
    ],
  },
  {
    path: 'audit',
    element: <AuditLogsPage />,
    menu: { label: '审计日志', icon: <IconList /> },
  },
  {
    path: 'settings',
    element: <SettingsPage />,
    menu: {
      label: '系统设置',
      icon: <IconSettings />,
      matchAliases: ['/system-info'],
    },
  },
  {
    path: 'system-info',
    element: <Navigate to="/settings" replace />,
  },
]

export interface AppMenuItem extends MenuMetadata {
  key: string
}

function absolutePath(path: string) {
  return `/${path.replace(/^\/+/, '')}`
}

export const appMenuItems: readonly AppMenuItem[] = appRoutes.flatMap((route) =>
  route.menu ? [{ key: absolutePath(route.path), ...route.menu }] : [],
)

function matchesMenuItem(pathname: string, item: AppMenuItem) {
  const prefixes = [item.key, ...(item.matchAliases ?? [])]
  return prefixes.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`))
}

export function resolveSelectedMenuKey(pathname: string) {
  const match = appMenuItems
    .filter((item) => matchesMenuItem(pathname, item))
    .sort((left, right) => right.key.length - left.key.length)[0]
  return match?.key ?? pathname
}
