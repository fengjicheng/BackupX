import { Avatar, Button, Dropdown, Menu } from '@arco-design/web-react'
import { useState } from 'react'
import { IconDown, IconLock, IconPoweroff, IconSafe } from '../icons'
import type { UserInfo } from '../../services/auth'
import { useAuthStore } from '../../stores/auth'
import { roleLabel } from '../../utils/permissions'
import { ChangePasswordModal } from './ChangePasswordModal'
import { MfaSettingsModal } from './MfaSettingsModal'

interface AccountControlsProps {
  user: UserInfo | null
}

export function AccountControls({ user }: AccountControlsProps) {
  const [passwordVisible, setPasswordVisible] = useState(false)
  const [securityVisible, setSecurityVisible] = useState(false)
  const logout = useAuthStore((state) => state.logout)

  const droplist = (
    <Menu
      onClickMenuItem={(key) => {
        if (key === 'password') {
          setPasswordVisible(true)
        } else if (key === 'two-factor') {
          setSecurityVisible(true)
        } else if (key === 'logout') {
          logout()
        }
      }}
    >
      <Menu.Item key="password">
        <IconLock style={{ marginRight: 8 }} />
        修改密码
      </Menu.Item>
      <Menu.Item key="two-factor">
        <IconSafe style={{ marginRight: 8 }} />
        多因素认证
      </Menu.Item>
      <Menu.Item key="logout">
        <IconPoweroff style={{ marginRight: 8 }} />
        退出登录
      </Menu.Item>
    </Menu>
  )

  return (
    <>
      <Dropdown droplist={droplist} position="br">
        <Button type="text" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Avatar size={28} style={{ backgroundColor: 'var(--color-primary-6)' }}>
            {(user?.displayName ?? user?.username ?? '管')[0]}
          </Avatar>
          <span>{user?.displayName ?? user?.username ?? '管理员'}</span>
          <span style={{ color: 'var(--color-text-3)', fontSize: 12 }}>
            [{roleLabel(user?.role)}]
          </span>
          <IconDown />
        </Button>
      </Dropdown>

      {passwordVisible ? (
        <ChangePasswordModal username={user?.username} onClose={() => setPasswordVisible(false)} />
      ) : null}
      {securityVisible ? (
        <MfaSettingsModal user={user} onClose={() => setSecurityVisible(false)} />
      ) : null}
    </>
  )
}
