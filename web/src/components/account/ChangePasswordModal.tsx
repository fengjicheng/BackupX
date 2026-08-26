import { Form, Input, Message, Modal } from '@arco-design/web-react'
import { useState } from 'react'
import {
  changePassword,
  clearTrustedDeviceToken,
  type ChangePasswordPayload,
} from '../../services/auth'
import { resolveErrorMessage } from '../../utils/error'

interface ChangePasswordModalProps {
  username?: string
  onClose: () => void
}

export function ChangePasswordModal({ username, onClose }: ChangePasswordModalProps) {
  const [loading, setLoading] = useState(false)
  const [form] = Form.useForm<ChangePasswordPayload & { confirmPassword: string }>()

  function close() {
    form.resetFields()
    onClose()
  }

  async function handleChangePassword() {
    try {
      const values = await form.validate()
      if (values.newPassword !== values.confirmPassword) {
        Message.error('两次输入的新密码不一致')
        return
      }
      setLoading(true)
      await changePassword({ oldPassword: values.oldPassword, newPassword: values.newPassword })
      clearTrustedDeviceToken(username)
      Message.success('密码修改成功')
      close()
    } catch (error) {
      if (error) {
        Message.error(resolveErrorMessage(error, '密码修改失败'))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title="修改密码"
      visible
      onCancel={close}
      onOk={handleChangePassword}
      confirmLoading={loading}
      unmountOnExit
    >
      <Form form={form} layout="vertical">
        <Form.Item field="oldPassword" label="当前密码" rules={[{ required: true, minLength: 8 }]}>
          <Input.Password placeholder="请输入当前密码" />
        </Form.Item>
        <Form.Item field="newPassword" label="新密码" rules={[{ required: true, minLength: 8 }]}>
          <Input.Password placeholder="请输入新密码（至少 8 位）" />
        </Form.Item>
        <Form.Item
          field="confirmPassword"
          label="确认新密码"
          rules={[{ required: true, minLength: 8 }]}
        >
          <Input.Password placeholder="请再次输入新密码" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
