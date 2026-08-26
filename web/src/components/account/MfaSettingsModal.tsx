import {
  Alert,
  Button,
  Divider,
  Form,
  Input,
  Message,
  Modal,
  Space,
  Tag,
  Typography,
} from '@arco-design/web-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  beginWebAuthnRegistration,
  clearTrustedDeviceToken,
  configureOtp,
  deleteWebAuthnCredential,
  disableTwoFactor,
  enableTwoFactor,
  finishWebAuthnRegistration,
  listTrustedDevices,
  listWebAuthnCredentials,
  prepareTwoFactor,
  regenerateRecoveryCodes,
  revokeTrustedDevice,
  type TrustedDevice,
  type TwoFactorSetupResult,
  type UserInfo,
  type WebAuthnCredential,
} from '../../services/auth'
import { useAuthStore } from '../../stores/auth'
import { resolveErrorMessage } from '../../utils/error'
import { createWebAuthnCredential } from '../../utils/webauthn'

interface MfaSettingsModalProps {
  user: UserInfo | null
  onClose: () => void
}

export function MfaSettingsModal({ user, onClose }: MfaSettingsModalProps) {
  const [loading, setLoading] = useState(false)
  const [setup, setSetup] = useState<TwoFactorSetupResult | null>(null)
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [webAuthnCredentials, setWebAuthnCredentials] = useState<WebAuthnCredential[]>([])
  const [trustedDevices, setTrustedDevices] = useState<TrustedDevice[]>([])
  const [detailsLoading, setDetailsLoading] = useState(false)
  const [form] = Form.useForm<{
    currentPassword: string
    code: string
    email: string
    phone: string
  }>()
  const setUser = useAuthStore((state) => state.setUser)
  const initializedRef = useRef(false)

  const loadSecurityDetails = useCallback(async () => {
    setDetailsLoading(true)
    try {
      const [credentials, devices] = await Promise.all([
        listWebAuthnCredentials(),
        listTrustedDevices(),
      ])
      setWebAuthnCredentials(credentials)
      setTrustedDevices(devices)
    } catch (error) {
      Message.error(resolveErrorMessage(error, '加载安全配置失败'))
    } finally {
      setDetailsLoading(false)
    }
  }, [])

  useEffect(() => {
    if (initializedRef.current) {
      return
    }
    initializedRef.current = true
    form.setFieldValue('email', user?.email ?? '')
    form.setFieldValue('phone', user?.phone ?? '')
    void loadSecurityDetails()
  }, [form, loadSecurityDetails, user?.email, user?.phone])

  function applySecurityUserUpdate(updated: UserInfo) {
    setUser(updated)
    if (!updated.mfaEnabled) {
      clearTrustedDeviceToken(updated.username)
    }
  }

  async function copyRecoveryCodes() {
    if (recoveryCodes.length === 0) return
    try {
      await navigator.clipboard.writeText(recoveryCodes.join('\n'))
      Message.success('已复制到剪贴板')
    } catch {
      Message.info('请手动选择文本复制')
    }
  }

  async function handleTwoFactorSetupAction() {
    try {
      const values = await form.validate()
      setLoading(true)
      if (!setup) {
        const result = await prepareTwoFactor({ currentPassword: values.currentPassword })
        setSetup(result)
        Message.success('TOTP 密钥已生成')
        return
      }
      const result = await enableTwoFactor({ code: values.code })
      setUser(result.user)
      setRecoveryCodes(result.recoveryCodes)
      Message.success('TOTP 已启用')
    } catch (error) {
      if (error) {
        Message.error(resolveErrorMessage(error, 'TOTP 操作失败'))
      }
    } finally {
      setLoading(false)
    }
  }

  async function handleRegenerateRecoveryCodes() {
    try {
      const values = await form.validate()
      setLoading(true)
      const result = await regenerateRecoveryCodes({
        currentPassword: values.currentPassword,
        code: values.code,
      })
      setUser(result.user)
      setRecoveryCodes(result.recoveryCodes)
      form.resetFields()
      Message.success('恢复码已重新生成')
    } catch (error) {
      if (error) {
        Message.error(resolveErrorMessage(error, '恢复码生成失败'))
      }
    } finally {
      setLoading(false)
    }
  }

  async function handleDisableTwoFactor() {
    try {
      const values = await form.validate()
      setLoading(true)
      const updated = await disableTwoFactor({
        currentPassword: values.currentPassword,
        code: values.code,
      })
      applySecurityUserUpdate(updated)
      Message.success('TOTP 已关闭')
      onClose()
    } catch (error) {
      if (error) {
        Message.error(resolveErrorMessage(error, '关闭 TOTP 失败'))
      }
    } finally {
      setLoading(false)
    }
  }

  function readCurrentPassword() {
    const currentPassword = String(form.getFieldValue('currentPassword') ?? '')
    if (currentPassword.trim().length < 8) {
      Message.error('请输入当前密码')
      return ''
    }
    return currentPassword
  }

  async function handleRegisterWebAuthn() {
    const currentPassword = readCurrentPassword()
    if (!currentPassword) return
    try {
      setLoading(true)
      const options = await beginWebAuthnRegistration({ currentPassword })
      const credential = await createWebAuthnCredential(options)
      const updated = await finishWebAuthnRegistration({
        name: navigator.userAgent.slice(0, 120),
        credential,
      })
      applySecurityUserUpdate(updated)
      await loadSecurityDetails()
      Message.success('通行密钥已注册')
    } catch (error) {
      Message.error(resolveErrorMessage(error, '通行密钥注册失败'))
    } finally {
      setLoading(false)
    }
  }

  async function handleDeleteWebAuthnCredential(id: string) {
    const currentPassword = readCurrentPassword()
    if (!currentPassword) return
    try {
      setLoading(true)
      const updated = await deleteWebAuthnCredential(id, { currentPassword })
      applySecurityUserUpdate(updated)
      await loadSecurityDetails()
      Message.success('通行密钥已删除')
    } catch (error) {
      Message.error(resolveErrorMessage(error, '删除通行密钥失败'))
    } finally {
      setLoading(false)
    }
  }

  async function handleConfigureOtp(channel: 'email' | 'sms', enabled: boolean) {
    const currentPassword = readCurrentPassword()
    if (!currentPassword) return
    const email = String(form.getFieldValue('email') ?? '')
    const phone = String(form.getFieldValue('phone') ?? '')
    try {
      setLoading(true)
      const updated = await configureOtp({ currentPassword, channel, enabled, email, phone })
      applySecurityUserUpdate(updated)
      form.setFieldValue('email', updated.email ?? '')
      form.setFieldValue('phone', updated.phone ?? '')
      Message.success(enabled ? 'OTP 已启用' : 'OTP 已关闭')
    } catch (error) {
      Message.error(resolveErrorMessage(error, 'OTP 配置失败'))
    } finally {
      setLoading(false)
    }
  }

  async function handleRevokeTrustedDevice(id: string) {
    const currentPassword = readCurrentPassword()
    if (!currentPassword) return
    try {
      setLoading(true)
      await revokeTrustedDevice(id, { currentPassword })
      clearTrustedDeviceToken(user?.username)
      await loadSecurityDetails()
      Message.success('可信设备已移除')
    } catch (error) {
      Message.error(resolveErrorMessage(error, '移除可信设备失败'))
    } finally {
      setLoading(false)
    }
  }

  function renderFooter() {
    if (recoveryCodes.length > 0) {
      return (
        <Space>
          <Button onClick={() => void copyRecoveryCodes()}>复制恢复码</Button>
          <Button type="primary" onClick={onClose}>
            完成
          </Button>
        </Space>
      )
    }
    if (user?.twoFactorEnabled) {
      return (
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button loading={loading} onClick={() => void handleRegenerateRecoveryCodes()}>
            重新生成恢复码
          </Button>
          <Button status="danger" loading={loading} onClick={() => void handleDisableTwoFactor()}>
            关闭 TOTP
          </Button>
        </Space>
      )
    }
    return (
      <Space>
        <Button onClick={onClose}>取消</Button>
        <Button type="primary" loading={loading} onClick={() => void handleTwoFactorSetupAction()}>
          {setup ? '启用 TOTP' : '生成 TOTP 二维码'}
        </Button>
      </Space>
    )
  }

  return (
    <Modal title="多因素认证" visible onCancel={onClose} footer={renderFooter()} unmountOnExit>
      {recoveryCodes.length > 0 ? (
        <Space direction="vertical" size="medium" style={{ width: '100%' }}>
          <Alert
            type="warning"
            content="恢复码只会显示一次。请立即保存；每个恢复码只能使用一次。"
          />
          <Input.TextArea value={recoveryCodes.join('\n')} autoSize readOnly />
        </Space>
      ) : (
        <Form form={form} layout="vertical">
          {user?.twoFactorEnabled ? (
            <>
              <Alert
                type="success"
                content={`当前账号已启用 TOTP，恢复码剩余 ${user.twoFactorRecoveryCodesRemaining ?? 0} 个。`}
                style={{ marginBottom: 16 }}
              />
              <Form.Item
                field="currentPassword"
                label="当前密码"
                rules={[{ required: true, minLength: 8 }]}
              >
                <Input.Password placeholder="请输入当前密码" />
              </Form.Item>
              <Form.Item
                field="code"
                label="TOTP 验证码"
                rules={[{ required: true, minLength: 6, maxLength: 10 }]}
              >
                <Input placeholder="请输入 6 位验证码" maxLength={10} />
              </Form.Item>
            </>
          ) : (
            <>
              {!setup ? (
                <>
                  <Alert
                    type="info"
                    content="启用前需要验证当前密码。"
                    style={{ marginBottom: 16 }}
                  />
                  <Form.Item
                    field="currentPassword"
                    label="当前密码"
                    rules={[{ required: true, minLength: 8 }]}
                  >
                    <Input.Password placeholder="请输入当前密码" />
                  </Form.Item>
                </>
              ) : (
                <>
                  <Alert
                    type="warning"
                    content="密钥仅在本次启用流程中显示。启用后会生成一次性恢复码。"
                    style={{ marginBottom: 16 }}
                  />
                  <div style={{ display: 'flex', gap: 20, alignItems: 'center', marginBottom: 16 }}>
                    <img
                      src={setup.qrCodeDataUrl}
                      alt="TOTP 二维码"
                      style={{
                        width: 160,
                        height: 160,
                        border: '1px solid var(--color-border)',
                        borderRadius: 8,
                      }}
                    />
                    <Space direction="vertical" size={8} style={{ flex: 1, minWidth: 0 }}>
                      <Typography.Text type="secondary">手动密钥</Typography.Text>
                      <Input value={setup.secret} readOnly />
                    </Space>
                  </div>
                  <Form.Item
                    field="code"
                    label="TOTP 验证码"
                    rules={[{ required: true, minLength: 6, maxLength: 10 }]}
                  >
                    <Input placeholder="请输入 6 位验证码" maxLength={10} />
                  </Form.Item>
                </>
              )}
            </>
          )}
          <Divider />
          <Space direction="vertical" size="medium" style={{ width: '100%' }}>
            <Space style={{ justifyContent: 'space-between', width: '100%' }}>
              <Typography.Title heading={6} style={{ margin: 0 }}>
                通行密钥
              </Typography.Title>
              <Tag color={webAuthnCredentials.length > 0 ? 'green' : 'gray'} bordered>
                {webAuthnCredentials.length > 0 ? `${webAuthnCredentials.length} 个` : '未注册'}
              </Tag>
            </Space>
            <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
              支持浏览器 Passkey、平台验证器或安全密钥，用于登录时替代验证码。
            </Typography.Paragraph>
            <Button loading={loading} onClick={() => void handleRegisterWebAuthn()}>
              注册当前设备通行密钥
            </Button>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              {detailsLoading ? (
                <Typography.Text type="secondary">正在加载通行密钥...</Typography.Text>
              ) : null}
              {webAuthnCredentials.map((item) => (
                <div
                  key={item.id}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    gap: 12,
                    alignItems: 'center',
                    padding: '8px 0',
                    borderTop: '1px solid var(--color-border)',
                  }}
                >
                  <Space direction="vertical" size={2}>
                    <Typography.Text>{item.name}</Typography.Text>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {item.lastUsedAt ? `最近使用 ${item.lastUsedAt}` : `创建于 ${item.createdAt}`}
                    </Typography.Text>
                  </Space>
                  <Button
                    size="small"
                    status="danger"
                    onClick={() => void handleDeleteWebAuthnCredential(item.id)}
                  >
                    删除
                  </Button>
                </div>
              ))}
            </Space>
          </Space>
          <Divider />
          <Space direction="vertical" size="medium" style={{ width: '100%' }}>
            <Typography.Title heading={6} style={{ margin: 0 }}>
              邮件 / 短信 OTP
            </Typography.Title>
            <Alert
              type="info"
              content="邮件 OTP 使用已启用的 Email 通知配置发送；短信 OTP 使用 Webhook 通知配置发送，payload 会包含 phone/code/purpose 字段。"
            />
            <Space wrap>
              <Tag color={user?.emailOtpEnabled ? 'green' : 'gray'} bordered>
                邮件 OTP {user?.emailOtpEnabled ? '已启用' : '未启用'}
              </Tag>
              <Tag color={user?.smsOtpEnabled ? 'green' : 'gray'} bordered>
                短信 OTP {user?.smsOtpEnabled ? '已启用' : '未启用'}
              </Tag>
            </Space>
            <Form.Item field="email" label="邮箱">
              <Input placeholder="启用邮件 OTP 时填写" />
            </Form.Item>
            <Form.Item field="phone" label="手机号">
              <Input placeholder="启用短信 OTP 时填写" />
            </Form.Item>
            <Space wrap>
              <Button
                loading={loading}
                onClick={() => void handleConfigureOtp('email', !user?.emailOtpEnabled)}
              >
                {user?.emailOtpEnabled ? '关闭邮件 OTP' : '启用邮件 OTP'}
              </Button>
              <Button
                loading={loading}
                onClick={() => void handleConfigureOtp('sms', !user?.smsOtpEnabled)}
              >
                {user?.smsOtpEnabled ? '关闭短信 OTP' : '启用短信 OTP'}
              </Button>
            </Space>
          </Space>
          <Divider />
          <Space direction="vertical" size="medium" style={{ width: '100%' }}>
            <Space style={{ justifyContent: 'space-between', width: '100%' }}>
              <Typography.Title heading={6} style={{ margin: 0 }}>
                可信设备
              </Typography.Title>
              <Tag color={trustedDevices.length > 0 ? 'green' : 'gray'} bordered>
                {trustedDevices.length} 个
              </Tag>
            </Space>
            <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
              登录时勾选“信任此设备”后，30 天内该设备可在密码校验通过后跳过多因素验证。
            </Typography.Paragraph>
            <Space direction="vertical" size={8} style={{ width: '100%' }}>
              {trustedDevices.map((item) => (
                <div
                  key={item.id}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    gap: 12,
                    alignItems: 'center',
                    padding: '8px 0',
                    borderTop: '1px solid var(--color-border)',
                  }}
                >
                  <Space direction="vertical" size={2}>
                    <Typography.Text>{item.name}</Typography.Text>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      最近使用 {item.lastUsedAt || '-'}，到期 {item.expiresAt}
                    </Typography.Text>
                  </Space>
                  <Button
                    size="small"
                    status="danger"
                    onClick={() => void handleRevokeTrustedDevice(item.id)}
                  >
                    移除
                  </Button>
                </div>
              ))}
              {!detailsLoading && trustedDevices.length === 0 ? (
                <Typography.Text type="secondary">暂无可信设备</Typography.Text>
              ) : null}
            </Space>
          </Space>
        </Form>
      )}
    </Modal>
  )
}
