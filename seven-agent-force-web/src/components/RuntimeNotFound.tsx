import { Button, Result } from 'antd';
import { useNavigate } from 'react-router-dom';

export default function RuntimeNotFound() {
  const navigate = useNavigate();

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'grid',
        placeItems: 'center',
        padding: 24,
      }}
    >
      <Result
        status="404"
        title="页面不存在"
        subTitle="请求的页面不存在或当前运行模式未启用该功能。"
        extra={
          <Button type="primary" onClick={() => navigate('/')}>
            返回首页
          </Button>
        }
      />
    </div>
  );
}
