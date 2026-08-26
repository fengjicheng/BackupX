import { Button, Layout, Menu, Space, Typography } from '@arco-design/web-react'
import { useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { AccountControls } from '../components/account/AccountControls'
import { EventCenter } from '../components/common/EventCenter'
import { GlobalSearch } from '../components/common/GlobalSearch'
import { IconCloud, IconMenuFold, IconMenuUnfold } from '../components/icons'
import { appMenuItems, resolveSelectedMenuKey } from '../router/routes'
import { useAuthStore } from '../stores/auth'
import { isAdmin } from '../utils/permissions'

const Header = Layout.Header
const Sider = Layout.Sider
const Content = Layout.Content

export function AppLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.user)

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider collapsible collapsed={collapsed} trigger={null} breakpoint="lg" width={220}>
        <div style={{ padding: '20px 16px', display: 'flex', alignItems: 'center', gap: 10 }}>
          <IconCloud style={{ fontSize: 28, color: 'var(--color-primary-6)' }} />
          {!collapsed && (
            <Typography.Title heading={5} style={{ margin: 0, fontWeight: 700 }}>
              BackupX
            </Typography.Title>
          )}
        </div>
        <Menu
          selectedKeys={[resolveSelectedMenuKey(location.pathname)]}
          onClickMenuItem={(key) => navigate(key)}
        >
          {appMenuItems
            .filter((item) => !item.adminOnly || isAdmin(user))
            .map((item) => (
              <Menu.Item key={item.key}>
                {item.icon}
                {item.label}
              </Menu.Item>
            ))}
        </Menu>
        {!collapsed && (
          <div style={{ position: 'absolute', bottom: 16, left: 0, right: 0, textAlign: 'center' }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              v1.0.0
            </Typography.Text>
          </div>
        )}
      </Sider>
      <Layout>
        <Header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '0 20px',
            background: 'var(--color-bg-2)',
            borderBottom: '1px solid var(--color-border)',
          }}
        >
          <Space>
            <Button
              type="text"
              icon={collapsed ? <IconMenuUnfold /> : <IconMenuFold />}
              onClick={() => setCollapsed((value) => !value)}
            />
            <GlobalSearch />
          </Space>
          <Space>
            <EventCenter />
            <AccountControls user={user} />
          </Space>
        </Header>
        <Content style={{ padding: '24px', background: 'var(--color-fill-2)', overflow: 'auto' }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
