import { App as AntdApp, ConfigProvider, Modal as StaticModal, message as staticMessage, notification as staticNotification } from 'antd';
import { useEffect } from 'react';
import BaseLayout from '@/components/layout';
import AuthSessionSyncBridge from '@/components/providers/AuthSessionSyncBridge';
import ClientDataProvider from '@/components/providers/ClientDataProvider';
import GlobalChallengeProvider from '@/components/providers/GlobalChallengeProvider';
import InitialStateProvider from '@/components/providers/InitialStateProvider';
import ReactQueryProvider from '@/components/providers/ReactQueryProvider';
import NotificationRealtimeProvider from '@/components/notification/NotificationRealtimeProvider';

function AntdStaticBridge() {
  const { message, notification, modal } = AntdApp.useApp();

  useEffect(() => {
    Object.assign(staticMessage, message);
    Object.assign(staticNotification, notification);
    Object.assign(StaticModal, {
      info: modal.info,
      success: modal.success,
      error: modal.error,
      warning: modal.warning,
      confirm: modal.confirm,
    });
  }, [message, notification, modal]);

  return null;
}

export default function AppFrame({ children }: { children?: React.ReactNode }) {
  return (
    <ConfigProvider
      theme={{
        token: {
          borderRadius: 12,
          colorPrimary: '#007AFF',
          colorTextBase: '#1d1d1f',
          fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, sans-serif',
          colorBgContainer: 'rgba(255, 255, 255, 0.6)',
          boxShadow: '0 4px 30px rgba(0, 0, 0, 0.08)',
          colorBorder: 'rgba(0,0,0,0.08)',
        },
        components: {
          Button: {
            borderRadius: 14,
            controlHeight: 40,
            fontWeight: 500,
            boxShadow: '0 2px 8px rgba(0, 122, 255, 0.2)',
          },
          Card: {
            borderRadiusLG: 20,
            boxShadowTertiary: '0 10px 40px -10px rgba(0,0,0,0.05)',
          },
          Input: {
            borderRadius: 12,
            controlHeight: 42,
            colorBgContainer: 'rgba(255, 255, 255, 0.5)',
            activeBorderColor: '#007AFF',
          },
          Menu: {
            itemBorderRadius: 10,
            itemHeight: 44,
            itemMarginInline: 12,
            itemSelectedBg: '#e6f4ff',
            itemSelectedColor: '#007AFF',
          },
          Layout: {
            bodyBg: 'transparent',
            headerBg: 'transparent',
            siderBg: 'transparent',
          },
          Statistic: {
            contentFontSize: 28,
            titleFontSize: 13,
          },
        },
      }}
    >
      <AntdApp>
        <AntdStaticBridge />
        <ReactQueryProvider>
          <AuthSessionSyncBridge />
          <InitialStateProvider>
            <NotificationRealtimeProvider>
              <ClientDataProvider>
                <GlobalChallengeProvider>
                  <BaseLayout>
                    {children}
                  </BaseLayout>
                </GlobalChallengeProvider>
              </ClientDataProvider>
            </NotificationRealtimeProvider>
          </InitialStateProvider>
        </ReactQueryProvider>
      </AntdApp>
    </ConfigProvider>
  );
}
