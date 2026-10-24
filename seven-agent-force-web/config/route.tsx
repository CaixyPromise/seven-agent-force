import type { ReactNode } from 'react';
import {
  CrownFilled,
  RadarChartOutlined,
  UserOutlined,
  TeamOutlined,
  MenuOutlined,
  ApartmentOutlined,
  BankOutlined,
  ClusterOutlined,
  IdcardOutlined,
  SafetyOutlined,
  FileTextOutlined,
  GlobalOutlined,
  SettingOutlined,
  DeploymentUnitOutlined,
  BellOutlined,
  HomeOutlined,
  ProjectOutlined,
} from '@ant-design/icons';
import {
  ADMIN_PERMISSIONS,
  DOCKER_PERMISSIONS,
  EXTERNAL_LOGIN_PERMISSIONS,
  FILE_PERMISSIONS,
  MENU_PERMISSIONS,
  ROLE_PERMISSIONS,
  SSO_CLIENT_PERMISSIONS,
  USER_PERMISSIONS,
} from '@/lib/auth/permissionCodes';


export interface AppRoute {
  key?: string;
  path: string;
  name: string;
  icon?: ReactNode;
  login?: boolean;
  component?: string | ReactNode;  // 用来和实际 Next.js 页面对应
  layout?: boolean;    // true=有全局Layout，false=无Layout
  access?: string;     // 权限点
  requiredPermissions?: string[];
  permissionMatchMode?: 'any' | 'all';
  featureCode?: string;
  routes?: AppRoute[]; // 子路由
  redirect?: string; // 重定向
  hideInMenu?: boolean; // 隐藏在侧边栏
}

export const routes: AppRoute[] = [
  {
    path: '/workbench',
    name: '工作台',
    icon: <ProjectOutlined />,
    routes: [
      {
        path: '/',
        name: '工作台',
        icon: <HomeOutlined />,
        component: './Home',
        requiredPermissions: ['workforce:task:list'],
      },
      {
        path: '/workforce/projects',
        name: '项目',
        icon: <ProjectOutlined />,
        component: './Workforce/Projects',
        requiredPermissions: ['workforce:task:list'],
      },
      {
        path: '/workforce/tasks',
        name: '任务',
        icon: <DeploymentUnitOutlined />,
        component: './Workforce/Tasks',
        requiredPermissions: ['workforce:task:list'],
      },
      {
        path: '/workforce/actions',
        name: '待我处理',
        icon: <BellOutlined />,
        component: './Workforce/Actions',
        requiredPermissions: ['workforce:task:list'],
      },
    ],
  },
  {
    path: '/workforce/projects/new',
    name: '新建项目',
    component: './Workforce/ProjectCreate',
    hideInMenu: true,
    requiredPermissions: ['workforce:task:create'],
  },
  {
    path: '/workforce/projects/:projectId',
    name: '项目详情',
    component: './Workforce/ProjectDetail',
    hideInMenu: true,
    requiredPermissions: ['workforce:task:query'],
  },
  {
    path: '/workforce/projects/:projectId/tasks/new',
    name: '发起任务',
    component: './Workforce/ProjectTaskCreate',
    hideInMenu: true,
    requiredPermissions: ['workforce:task:create'],
  },
  {
    path: '/workforce/tasks/new',
    name: '新建任务',
    component: './Workforce/TaskCreate',
    hideInMenu: true,
    requiredPermissions: ['workforce:task:create'],
  },
  {
    path: '/workforce/tasks/:taskId',
    name: 'Agent 任务详情',
    component: './Workforce/TaskDetail',
    hideInMenu: true,
    requiredPermissions: ['workforce:task:query'],
  },
  {
    path: '/system',
    name: '平台管理',
    routes: [
      {
        path: '/system',
        name: '系统控制台',
        icon: <CrownFilled />,
        routes: [
          {
            icon: <TeamOutlined />,
            path: '/system/identity',
            name: '身份与组织',
            routes: [
              {
                icon: <UserOutlined />,
                path: '/system/user',
                component: './System/User',
                name: '用户管理',
                requiredPermissions: [USER_PERMISSIONS.LIST],
              },
              {
                icon: <ApartmentOutlined />,
                path: '/system/organization-management',
                component: './System/OrganizationManagement',
                name: '组织架构管理',
                requiredPermissions: ['system:org:list', 'system:dept:list', 'system:post:list'],
              },
              {
                icon: <ApartmentOutlined />,
                path: '/system/organization',
                component: './System/Organization',
                name: '组织管理',
                requiredPermissions: ['system:org:list'],
              },
              {
                icon: <BankOutlined />,
                path: '/system/department',
                component: './System/Department',
                name: '部门管理',
                requiredPermissions: ['system:dept:list'],
              },
              {
                icon: <IdcardOutlined />,
                path: '/system/post',
                component: './System/Post',
                name: '岗位管理',
                requiredPermissions: ['system:post:list'],
              },
            ],
          },
          {
            icon: <SafetyOutlined />,
            path: '/system/access',
            name: '权限与认证',
            routes: [
              {
                icon: <TeamOutlined />,
                path: '/system/role',
                component: './System/Role',
                name: '角色管理',
                requiredPermissions: [ROLE_PERMISSIONS.LIST],
              },
              {
                icon: <MenuOutlined />,
                path: '/system/menu',
                component: './System/Menu',
                name: '菜单管理',
                requiredPermissions: [MENU_PERMISSIONS.LIST],
              },
              {
                icon: <SafetyOutlined />,
                path: '/system/permission',
                component: './System/Permission',
                name: '权限资源',
                requiredPermissions: ['system:permission:list'],
              },
              {
                icon: <SafetyOutlined />,
                path: '/system/sso-client',
                component: './System/SsoClient',
                name: 'OAuth 客户端',
                requiredPermissions: [SSO_CLIENT_PERMISSIONS.LIST],
              },
              {
                icon: <GlobalOutlined />,
                path: '/system/external-login-provider',
                component: './System/ExternalLoginProvider',
                name: '外部登录',
                requiredPermissions: [EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_LIST],
              },
            ],
          },
          {
            icon: <ClusterOutlined />,
            path: '/system/ops',
            name: '平台运维',
            routes: [
              {
                icon: <ClusterOutlined />,
                path: '/system/docker',
                component: './System/Docker',
                name: 'Docker 运维',
                requiredPermissions: [
                  DOCKER_PERMISSIONS.CONTAINER_LIST,
                  DOCKER_PERMISSIONS.CONTAINER_QUERY,
                  DOCKER_PERMISSIONS.IMAGE_LIST,
                  DOCKER_PERMISSIONS.REGISTRY_LIST,
                  DOCKER_PERMISSIONS.COMPOSE_VALIDATE,
                  DOCKER_PERMISSIONS.OPERATION_LIST,
                ],
                permissionMatchMode: 'any',
              },
              {
                icon: <RadarChartOutlined />,
                path: '/system/observability',
                component: './System/Observability',
                name: '可观测性中心',
                requiredPermissions: [ADMIN_PERMISSIONS.OBSERVABILITY_VIEW],
              },
              {
                icon: <GlobalOutlined />,
                path: '/system/online-user',
                component: './System/OnlineUser',
                name: '在线用户',
                requiredPermissions: [ADMIN_PERMISSIONS.ONLINE_VIEW],
              },
              {
                icon: <FileTextOutlined />,
                path: '/system/runtime-log',
                component: './System/RuntimeLog',
                name: '应用运行日志',
                requiredPermissions: [ADMIN_PERMISSIONS.RUNTIME_LOG_VIEW],
              },
            ],
          },
          {
            icon: <SettingOutlined />,
            path: '/system/settings',
            name: '配置与内容',
            routes: [
              {
                icon: <SettingOutlined />,
                path: '/system/config',
                component: './System/Config',
                name: '配置管理',
                requiredPermissions: ['system:config:query', 'system:config:group:query'],
                permissionMatchMode: 'all',
              },
              {
                icon: <SettingOutlined />,
                path: '/system/dict',
                component: './System/Dict',
                name: '字典管理',
                requiredPermissions: ['system:dict:query'],
              },
              {
                icon: <FileTextOutlined />,
                path: '/system/files',
                name: '文件管理',
                routes: [
                  {
                    path: '/system/files',
                    component: './System/Files',
                    name: '文件列表',
                    requiredPermissions: [FILE_PERMISSIONS.LIST],
                  },
                  {
                    path: '/system/file-tasks',
                    component: './System/FileTasks',
                    name: '任务处理',
                    requiredPermissions: [FILE_PERMISSIONS.TASK_LIST],
                  },
                  {
                    path: '/system/storage',
                    component: './System/Storage',
                    name: '存储策略',
                    requiredPermissions: [FILE_PERMISSIONS.STORAGE_LIST],
                  },
                ],
              },
            ],
          },
          {
            icon: <FileTextOutlined />,
            path: '/system/audit',
            name: '审计与日志',
            routes: [
              {
                icon: <FileTextOutlined />,
                path: '/system/operation-log',
                component: './System/OperationLog',
                name: '操作审计',
                requiredPermissions: [ADMIN_PERMISSIONS.LOG_VIEW],
              },
            ],
          },
          {
            icon: <UserOutlined />,
            path: '/system/user',
            component: './System/User',
            name: '用户管理',
            requiredPermissions: [USER_PERMISSIONS.LIST],
            hideInMenu: true,
          },
          {
            icon: <ApartmentOutlined />,
            path: '/system/organization-management',
            component: './System/OrganizationManagement',
            name: '组织架构管理',
            requiredPermissions: ['system:org:list', 'system:dept:list', 'system:post:list'],
            hideInMenu: true,
          },
          {
            icon: <TeamOutlined />,
            path: '/system/role',
            component: './System/Role',
            name: '角色管理',
            requiredPermissions: [ROLE_PERMISSIONS.LIST],
            hideInMenu: true,
          },
          {
            icon: <MenuOutlined />,
            path: '/system/menu',
            component: './System/Menu',
            name: '菜单管理',
            requiredPermissions: [MENU_PERMISSIONS.LIST],
            hideInMenu: true,
          },
          {
            icon: <ApartmentOutlined />,
            path: '/system/organization',
            component: './System/Organization',
            name: '组织管理',
            requiredPermissions: ['system:org:list'],
            hideInMenu: true,
          },
          {
            icon: <BankOutlined />,
            path: '/system/department',
            component: './System/Department',
            name: '部门管理',
            requiredPermissions: ['system:dept:list'],
            hideInMenu: true,
          },
          {
            icon: <IdcardOutlined />,
            path: '/system/post',
            component: './System/Post',
            name: '岗位管理',
            requiredPermissions: ['system:post:list'],
            hideInMenu: true,
          },
          {
            icon: <SafetyOutlined />,
            path: '/system/permission',
            component: './System/Permission',
            name: '权限管理',
            requiredPermissions: ['system:permission:list'],
            hideInMenu: true,
          },
          {
            icon: <RadarChartOutlined />,
            path: '/system/observability',
            component: './System/Observability',
            name: '可观测性中心',
            requiredPermissions: [ADMIN_PERMISSIONS.OBSERVABILITY_VIEW],
            hideInMenu: true,
          },
          {
            icon: <ClusterOutlined />,
            path: '/system/docker',
            component: './System/Docker',
            name: 'Docker',
            requiredPermissions: [
              DOCKER_PERMISSIONS.CONTAINER_LIST,
              DOCKER_PERMISSIONS.CONTAINER_QUERY,
              DOCKER_PERMISSIONS.IMAGE_LIST,
              DOCKER_PERMISSIONS.REGISTRY_LIST,
              DOCKER_PERMISSIONS.COMPOSE_VALIDATE,
              DOCKER_PERMISSIONS.OPERATION_LIST,
            ],
            permissionMatchMode: 'any',
            hideInMenu: true,
          },
          {
            icon: <SafetyOutlined />,
            path: '/system/security',
            name: '权限与认证',
            hideInMenu: true,
            routes: [
              {
                path: '/system/sso-client',
                component: './System/SsoClient',
                name: 'OAuth 客户端',
                requiredPermissions: [SSO_CLIENT_PERMISSIONS.LIST],
                hideInMenu: true,
              },
              {
                path: '/system/external-login-provider',
                component: './System/ExternalLoginProvider',
                name: '外部登录',
                requiredPermissions: [EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_LIST],
                hideInMenu: true,
              },
            ],
          },
          {
            icon: <SafetyOutlined />,
            path: '/system/config',
            component: './System/Config',
            name: '配置管理',
            requiredPermissions: ['system:config:query', 'system:config:group:query'],
            permissionMatchMode: 'all',
            hideInMenu: true,
          },
          {
            icon: <SafetyOutlined />,
            path: '/system/dict',
            component: './System/Dict',
            name: '字典管理',
            requiredPermissions: ['system:dict:query'],
            hideInMenu: true,
          },
          {
            icon: <FileTextOutlined />,
            path: '/system/operation-log',
            component: './System/OperationLog',
            name: '日志与审计',
            requiredPermissions: [ADMIN_PERMISSIONS.LOG_VIEW],
            hideInMenu: true,
          },
          {
            icon: <FileTextOutlined />,
            path: '/system/runtime-log',
            component: './System/RuntimeLog',
            name: '应用运行日志',
            requiredPermissions: [ADMIN_PERMISSIONS.RUNTIME_LOG_VIEW],
            hideInMenu: true,
          },
          {
            icon: <GlobalOutlined />,
            path: '/system/online-user',
            component: './System/OnlineUser',
            name: '在线用户',
            requiredPermissions: [ADMIN_PERMISSIONS.ONLINE_VIEW],
            hideInMenu: true,
          },
          {
            icon: <FileTextOutlined />,
            path: '/system/files',
            name: '文件管理',
            hideInMenu: true,
            routes: [
              {
                path: '/system/files',
                component: './System/Files',
                name: '文件列表',
                requiredPermissions: [FILE_PERMISSIONS.LIST],
              },
              {
                path: '/system/file-tasks',
                component: './System/FileTasks',
                name: '任务处理',
                requiredPermissions: [FILE_PERMISSIONS.TASK_LIST],
              },
              {
                path: '/system/storage',
                component: './System/Storage',
                name: '存储策略',
                requiredPermissions: [FILE_PERMISSIONS.STORAGE_LIST],
              },
            ],
          },
        ]
      }
    ],
  },
  {
    path: '/account/settings',
    name: '个人中心',
    icon: <UserOutlined />,
    component: './Account/Settings',
    hideInMenu: true,
  },
  {
    path: '/notifications',
    name: '消息',
    component: './Notifications',
    hideInMenu: true,
  },
];
