"use client"
import {
  GithubFilled,
  LeftOutlined,
  LogoutOutlined,
  RightOutlined,
} from '@ant-design/icons';
import {ProLayout, ProSettings} from '@ant-design/pro-components';
import {
  PageContainer,
} from '@ant-design/pro-components';
import { Button, ConfigProvider, Dropdown, Result, Spin, message } from 'antd';
import type { MenuProps } from 'antd';
import React, {useMemo} from 'react';
import { useQuery } from '@tanstack/react-query';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import {routes} from "config/route";
import type { AppRoute } from 'config/route';
import { getCurrentUserMenus } from '@/api/authController';
import { useUIStore } from '@/store/ui';
import { useAuthStore } from '@/store/auth';
import { isRouteWithoutLayout } from '@/lib/config/constants';
import { buildLoginRedirectUrl } from '@/lib/auth/navigation';
import { requiresAuth } from '@/lib/auth/routes';
import {
  canAccessRoute,
  filterRoutesByAccess,
  findFirstAccessiblePath,
  findRouteChain,
} from '@/lib/navigation/routeAccess';
import { buildRoutesFromMenuTree, getStaticNavigationRoutes, getStaticSupportRoutes, mergeDynamicMenuWithStaticFallback } from '@/lib/navigation/menuRoutes';
import useConfig from "@/hooks/useConfig";
import useConfigValue from "@/hooks/useConfigValue";
import { AUTH_MENUS_QUERY_KEY, useLogoutMutation } from '@/hooks/useAuth';
import { useAuth } from '@/hooks/auth';
import { useSafeRuntimeFeatures } from '@/hooks/useRuntimeFeatures';
import { pixelAvatarDataUrl } from '@/components/user/pixelAvatarDataUrl';
import NotificationBell from '@/components/notification/NotificationBell';

const THEME_PRESET_COLORS: Record<string, string> = {
  blue: '#007AFF',
  green: '#16A34A',
  purple: '#7C3AED',
  orange: '#EA580C',
};

const GlobalLayoutShell = ({
  children,
  pathname,
  system,
  user,
  collapsed,
  toggleSider,
  navigate,
  routeConfig,
}: {
  children: React.ReactNode;
  pathname: string;
  system: ReturnType<typeof useConfig>['system'];
  user: ReturnType<typeof useAuthStore.getState>['user'];
  collapsed: boolean;
  toggleSider: (collapsed?: boolean) => void;
  navigate: ReturnType<typeof useNavigate>;
  routeConfig: AppRoute[];
}) => {
  const settings: Partial<ProSettings> = {
    layout: 'side',
  };
  const title = useConfigValue<string>('SEVEN_FRONTEND_METADATA.title');
  const shortTitle = useConfigValue<string>('SEVEN_FRONTEND_METADATA.shortTitle');
  const themePreset = useConfigValue<string>('SEVEN_FRONTEND_METADATA.themePrimaryColor');
  const resolvedTitle = title?.value || 'Seven Agent Force';
  const resolvedShortTitle = shortTitle?.value || resolvedTitle;
  const primaryColor =
    THEME_PRESET_COLORS[String(themePreset?.value ?? '').trim().toLowerCase()] ??
    THEME_PRESET_COLORS.blue;
  const hidePageHeaderTitle =
    pathname === '/' || pathname === '/system/docker' || pathname.startsWith('/system/docker/');
  const logoutMutation = useLogoutMutation();
  const avatarSeed = user?.username || user?.nickname || 'seven';
  const avatarMenuItems: MenuProps['items'] = [
    {
      key: 'account-settings',
      label: '个人中心',
    },
    {
      type: 'divider',
    },
    {
      key: 'logout',
      label: '退出登录',
      icon: <LogoutOutlined />,
      danger: true,
    },
  ];

  const handleLogout = async () => {
    await logoutMutation.mutateAsync();
    message.success('已退出登录');
    navigate('/login', { replace: true });
  };

  const brandMark = (
    <span
      aria-hidden="true"
      style={{
        display: 'inline-grid',
        width: 32,
        height: 32,
        minWidth: 32,
        placeItems: 'center',
        borderRadius: 10,
        background: `linear-gradient(135deg, ${primaryColor} 0%, #22d3ee 100%)`,
        boxShadow: '0 8px 18px -12px rgba(14, 116, 144, 0.72)',
        color: '#ffffff',
        fontSize: 16,
        fontWeight: 800,
        lineHeight: 1,
      }}
    >
      7
    </span>
  );

  return (
    <ConfigProvider theme={{ token: { colorPrimary: primaryColor } }}>
      <div
        style={{
          height: '100vh',
          minWidth: 0,
        }}
      >
        <ProLayout
          siderWidth={256}
          token={{
            sider: {
              colorBgMenuItemSelected: `${primaryColor}14`,
              colorTextMenuSelected: primaryColor,
            },
          }}
          bgLayoutImgList={[]}
          route={{ path: '/', routes: routeConfig }}
          appList={system.appList || []}
          logo={brandMark}
          title={resolvedTitle}
          menuHeaderRender={(logoDom) => (
            <div
              style={{
                display: 'flex',
                minWidth: 0,
                height: 48,
                alignItems: 'center',
                justifyContent: collapsed ? 'center' : undefined,
                gap: 10,
                overflow: 'hidden',
              }}
            >
              {logoDom}
              {!collapsed ? (
                <span
                  title={resolvedShortTitle}
                  style={{
                    minWidth: 0,
                    overflow: 'hidden',
                    color: '#172033',
                    fontSize: 16,
                    fontWeight: 700,
                    lineHeight: 1.25,
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {resolvedTitle}
                </span>
              ) : null}
            </div>
          )}
          location={{
            pathname,
          }}
          menu={{
            type: 'group',
          }}
          menuRender={(_, dom) => (
            <ConfigProvider theme={{ components: { Menu: { itemMarginInline: collapsed ? 2 : 12, itemBorderRadius: 10 } } }}>
              {dom}
            </ConfigProvider>
          )}
          collapsed={collapsed}
          onCollapse={(value) => toggleSider(value)}
          collapsedButtonRender={(isCollapsed) => (
            <Button
              type="default"
              shape="circle"
              size="small"
              icon={isCollapsed ? <RightOutlined /> : <LeftOutlined />}
              aria-label={isCollapsed ? '展开导航' : '收起导航'}
              title={isCollapsed ? '展开导航' : '收起导航'}
              aria-expanded={!isCollapsed}
              onClick={() => toggleSider(!isCollapsed)}
              style={{ position: 'absolute', top: 20, insetInlineEnd: -16, width: 32, height: 32, zIndex: 1 }}
            />
          )}
          avatarProps={{
            src: user?.userAvatar || pixelAvatarDataUrl(avatarSeed),
            title: user?.nickname ?? user?.username ?? '未登录',
            size: 'small',
            render: (_, avatarDom) => (
            <div
              style={{
                display: 'flex',
                width: '100%',
                minWidth: 0,
                alignItems: 'center',
                flexDirection: collapsed ? 'column' : 'row',
                gap: collapsed ? 8 : 0,
              }}
            >
              <Dropdown
                menu={{
                  items: avatarMenuItems,
                  onClick: async ({ key }) => {
                    if (key === 'account-settings') {
                      navigate('/account/settings');
                      return;
                    }
                    if (key === 'logout') {
                      await handleLogout();
                    }
                  },
                }}
                trigger={['click']}
              >
                <Button
                  type="text"
                  aria-label="个人菜单"
                  title="个人菜单"
                  style={{ display: 'inline-flex', flex: '0 0 auto', padding: 0, width: collapsed ? 32 : undefined, height: 32 }}
                >
                  {avatarDom}
                </Button>
              </Dropdown>
              <span
                style={{
                  display: 'inline-flex',
                  flex: '0 0 auto',
                  alignItems: 'center',
                  flexDirection: collapsed ? 'column' : 'row',
                  marginLeft: collapsed ? 0 : 'auto',
                  gap: collapsed ? 8 : 0,
                }}
              >
                <NotificationBell />
                <Button
                  type="text"
                  shape="circle"
                  size="small"
                  icon={<GithubFilled />}
                  href="https://www.github.com/CaixyPromise"
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label="打开 GitHub"
                  title="GitHub"
                />
              </span>
            </div>
          ),
          }}
          actionsRender={() => []}
          style={{ minWidth: 0 }}
          menuItemRender={(item, dom) => (
          <div
            role="button"
            tabIndex={0}
            aria-label={typeof item.name === 'string' ? item.name : undefined}
            aria-current={pathname === item.path ? 'page' : undefined}
            title={collapsed && typeof item.name === 'string' ? item.name : undefined}
            style={{
              display: 'flex',
              width: '100%',
              minHeight: '100%',
              alignItems: 'center',
              cursor: item.path ? 'pointer' : 'default',
            }}
            onClick={() => {
              if (item.path) {
                if (/^https?:\/\//.test(item.path)) {
                  window.open(item.path, '_blank', 'noopener,noreferrer');
                  return;
                }
                navigate(item.path);
              }
            }}
            onKeyDown={(event) => {
              if (!item.path) {
                return;
              }
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                if (/^https?:\/\//.test(item.path)) {
                  window.open(item.path, '_blank', 'noopener,noreferrer');
                  return;
                }
                navigate(item.path);
              }
            }}
          >
            {dom}
          </div>
          )}
          {...settings}
        >
          <PageContainer
            className={pathname === '/' ? 'workforce-workbench-page-container' : undefined}
            title={hidePageHeaderTitle ? false : undefined}
            style={{
              padding: '16px',
              minHeight: '100vh',
              width: '100%',
              minWidth: 0,
              maxWidth: '100%',
              overflow: 'visible',
              background: '#f6f8fc',
            }}
          >
            <div
              style={{
                width: '100%',
                minWidth: 0,
                maxWidth: '100%',
                overflow: 'visible',
                background: '#f6f8fc',
              }}
            >
              {children}
            </div>
          </PageContainer>
        </ProLayout>
      </div>
    </ConfigProvider>
  );
};

const BaseLayout = ({children} : {children : React.ReactNode}) => {
  const location = useLocation();
  const navigate = useNavigate();
  const pathname = location.pathname;
  const {system} = useConfig();
  const collapsed = useUIStore((state) => state.siderCollapsed);
  const toggleSider = useUIStore((state) => state.toggleSider);
  const user = useAuthStore((state) => state.user);
  const { isAdmin, isAuthenticated, permissions } = useAuth();
  const runtimeFeaturesQuery = useSafeRuntimeFeatures({ enabled: isAuthenticated });
  const runtimeFeatures = runtimeFeaturesQuery.safeData;
  const menuQuery = useQuery({
    queryKey: AUTH_MENUS_QUERY_KEY,
    queryFn: async () => {
      const response = await getCurrentUserMenus({ skipAuthRedirect: true });
      return Array.isArray(response.data) ? response.data : [];
    },
    enabled: isAuthenticated,
    staleTime: 5 * 60 * 1000,
    retry: 0,
  });
  const menuRoutes = useMemo(
    () => buildRoutesFromMenuTree(menuQuery.data, runtimeFeatures),
    [menuQuery.data, runtimeFeatures],
  );
  const staticNavigationRoutes = useMemo(
    () => getStaticNavigationRoutes(routes, runtimeFeatures),
    [runtimeFeatures],
  );
  const isDynamicProtectedPath = pathname.startsWith('/system') || pathname.startsWith('/admin');
  const shouldUseDynamicMenus = isAuthenticated && isDynamicProtectedPath;
  const effectiveRoutes = useMemo(() => {
    if (isAuthenticated) {
      return [
        ...mergeDynamicMenuWithStaticFallback(menuRoutes, staticNavigationRoutes),
        ...getStaticSupportRoutes(routes, runtimeFeatures),
      ];
    }
    return routes;
  }, [isAuthenticated, menuRoutes, runtimeFeatures, staticNavigationRoutes]);
  const accessContext = useMemo(
    () => ({ isAdmin, permissions }),
    [isAdmin, permissions],
  );

  const currentRouteChain = useMemo(
    () => findRouteChain(pathname, effectiveRoutes) ?? [],
    [effectiveRoutes, pathname],
  );
  const currentRoute = currentRouteChain.at(-1) ?? null;
  const useGlobalLayout =
    currentRoute?.layout !== false && !isRouteWithoutLayout(pathname);
  const routeConfig = useMemo(
    () => filterRoutesByAccess(effectiveRoutes, accessContext),
    [accessContext, effectiveRoutes],
  );
  const hasRouteAccess =
    currentRouteChain.length > 0
      ? currentRouteChain.every((route) => canAccessRoute(route, accessContext))
      : !shouldUseDynamicMenus;
  const preferredPath = useMemo(
    () =>
      pathname.startsWith('/system')
        ? findFirstAccessiblePath(effectiveRoutes, accessContext) ?? '/'
        : '/',
    [accessContext, effectiveRoutes, pathname],
  );

  if (!useGlobalLayout) {
    return <>{children}</>;
  }

  if (!isAuthenticated && requiresAuth(pathname)) {
    return <Navigate to={buildLoginRedirectUrl(window.location.href)} replace />;
  }

  if (shouldUseDynamicMenus && menuQuery.isLoading) {
    return (
      <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}>
        <Spin description="正在加载菜单权限..." />
      </div>
    );
  }

  const unauthorizedFallback = (
    <Result
      status="403"
      title="访问被拒绝"
      subTitle="您没有权限访问当前页面，请联系管理员为您分配角色或权限。"
      extra={
        <Button type="primary" onClick={() => navigate(preferredPath)}>
          返回首页
        </Button>
      }
    />
  );

  const menuLoadErrorFallback = (
    <Result
      status="warning"
      title="菜单权限加载失败"
      subTitle="无法确认当前账号可访问的菜单，请刷新页面或重新登录后再试。"
      extra={
        <Button type="primary" onClick={() => menuQuery.refetch()}>
          重新加载
        </Button>
      }
    />
  );

  const content = menuQuery.isError && shouldUseDynamicMenus
    ? menuLoadErrorFallback
    : hasRouteAccess
      ? children
      : unauthorizedFallback;

  return (
    <GlobalLayoutShell
      pathname={pathname}
      system={system}
      user={user}
      collapsed={collapsed}
      toggleSider={toggleSider}
      navigate={navigate}
      routeConfig={routeConfig}
    >
      {content}
    </GlobalLayoutShell>
  );
};

export default BaseLayout;
