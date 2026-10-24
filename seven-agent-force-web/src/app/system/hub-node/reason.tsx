import { Input, Modal } from 'antd';
import { SafetyCertificateOutlined } from '@ant-design/icons';
import { runeLength } from './controllerContract';

export function requestActionReason(title: string, placeholder: string): Promise<string | null> {
  return new Promise((resolve) => {
    let reason = '';
    const instance = Modal.confirm({
      title,
      icon: <SafetyCertificateOutlined />,
      content: (
        <Input.TextArea
          data-testid="hub-action-reason"
          rows={3}
          count={{ max: 512, strategy: runeLength }}
          autoFocus
          placeholder={placeholder}
          onChange={(event) => {
            reason = event.target.value;
            instance.update({
              okButtonProps: { disabled: !reason.trim() || runeLength(reason) > 512 },
            });
          }}
        />
      ),
      okText: '确认执行',
      cancelText: '取消',
      okButtonProps: { disabled: true },
      onOk: () => {
        resolve(reason.trim());
      },
      onCancel: () => resolve(null),
    });
  });
}
