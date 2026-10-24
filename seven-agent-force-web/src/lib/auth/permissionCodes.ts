export const USER_PERMISSIONS = {
  LIST: 'system:user:list',
  QUERY: 'system:user:query',
  CREATE: 'system:user:create',
  UPDATE: 'system:user:update',
  DELETE: 'system:user:delete',
  STATUS: 'system:user:status',
  RESET_PASSWORD: 'system:user:reset-password',
  ACCESS_QUERY: 'system:user:access:query',
  ACCESS_EXPLAIN: 'system:user:access:explain',
} as const;

export const ADMIN_PERMISSIONS = {
  LOG_VIEW: 'admin:log:view',
  LOG_EXPORT: 'admin:log:export',
  LOG_CLEAN: 'admin:log:clean',
  LOG_DELETE: 'admin:log:delete',
  RUNTIME_LOG_VIEW: 'admin:runtime-log:view',
  RUNTIME_LOG_STREAM: 'admin:runtime-log:stream',
  ONLINE_VIEW: 'admin:online:view',
  ONLINE_STATS: 'admin:online:stats',
  ONLINE_KICK: 'admin:online:kick',
  OBSERVABILITY_VIEW: 'admin:observability:view',
} as const;

export const TEMPORARY_PERMISSION_PERMISSIONS = {
  QUERY: 'admin:temp-permission:query',
  GRANT: 'admin:temp-permission:grant',
  EXTEND: 'admin:temp-permission:extend',
  REVOKE: 'admin:temp-permission:revoke',
} as const;

export const FILE_PERMISSIONS = {
  LIST: 'system:file:list',
  QUERY: 'system:file:query',
  DELETE: 'system:file:delete',
  TASK_LIST: 'system:file-task:list',
  TASK_RETRY: 'system:file-task:retry',
  STORAGE_LIST: 'system:storage:list',
  STORAGE_ADD: 'system:storage:add',
  STORAGE_EDIT: 'system:storage:edit',
  STORAGE_DELETE: 'system:storage:delete',
} as const;

export const DOCKER_PERMISSIONS = {
  CONTAINER_LIST: 'admin:docker:container:list',
  CONTAINER_QUERY: 'admin:docker:container:query',
  CONTAINER_LOGS: 'admin:docker:container:logs',
  CONTAINER_TERMINAL: 'admin:docker:container:terminal',
  CONTAINER_START: 'admin:docker:container:start',
  CONTAINER_STOP: 'admin:docker:container:stop',
  CONTAINER_RESTART: 'admin:docker:container:restart',
  CONTAINER_DELETE: 'admin:docker:container:delete',
  CONTAINER_CREATE: 'admin:docker:container:create',
  IMAGE_LIST: 'admin:docker:image:list',
  IMAGE_QUERY: 'admin:docker:image:query',
  IMAGE_CONTAINERS: 'admin:docker:image:containers',
  IMAGE_PULL: 'admin:docker:image:pull',
  IMAGE_TAG: 'admin:docker:image:tag',
  IMAGE_PUSH: 'admin:docker:image:push',
  IMAGE_DELETE: 'admin:docker:image:delete',
  IMAGE_STARTUP_PREVIEW: 'admin:docker:image:startup-preview',
  REGISTRY_LIST: 'admin:docker:registry:list',
  REGISTRY_CREATE: 'admin:docker:registry:create',
  REGISTRY_UPDATE: 'admin:docker:registry:update',
  REGISTRY_TEST: 'admin:docker:registry:test',
  REGISTRY_DELETE: 'admin:docker:registry:delete',
  REGISTRY_SYNC: 'admin:docker:registry:sync',
  NETWORK_LIST: 'admin:docker:network:list',
  NETWORK_QUERY: 'admin:docker:network:query',
  NETWORK_CREATE: 'admin:docker:network:create',
  NETWORK_CONNECT: 'admin:docker:network:connect',
  NETWORK_DISCONNECT: 'admin:docker:network:disconnect',
  NETWORK_DELETE: 'admin:docker:network:delete',
  NETWORK_PRUNE: 'admin:docker:network:prune',
  VOLUME_LIST: 'admin:docker:volume:list',
  VOLUME_QUERY: 'admin:docker:volume:query',
  VOLUME_CREATE: 'admin:docker:volume:create',
  VOLUME_DELETE: 'admin:docker:volume:delete',
  VOLUME_PRUNE: 'admin:docker:volume:prune',
  CONFIG_QUERY: 'admin:docker:config:query',
  CONFIG_VALIDATE: 'admin:docker:config:validate',
  CONFIG_UPDATE: 'admin:docker:config:update',
  CONFIG_RESTART: 'admin:docker:config:restart',
  COMPOSE_PROJECT_LIST: 'admin:docker:compose:project:list',
  COMPOSE_PROJECT_CREATE: 'admin:docker:compose:project:create',
  COMPOSE_PROJECT_QUERY: 'admin:docker:compose:project:query',
  COMPOSE_PROJECT_UPDATE: 'admin:docker:compose:project:update',
  COMPOSE_VALIDATE: 'admin:docker:compose:validate',
  COMPOSE_UP: 'admin:docker:compose:up',
  OPERATION_LIST: 'admin:docker:operation:list',
  OPERATION_QUERY: 'admin:docker:operation:query',
  OPERATION_STREAM: 'admin:docker:operation:stream',
  OPERATION_CANCEL: 'admin:docker:operation:cancel',
  OPERATION_RETRY: 'admin:docker:operation:retry',
  DANGEROUS: 'admin:docker:dangerous',
  POLICY_OVERRIDE: 'admin:docker:policy:override',
} as const;

export const ROLE_PERMISSIONS = {
  LIST: 'system:role:list',
  QUERY: 'system:role:query',
  ADD: 'system:role:add',
  EDIT: 'system:role:edit',
  REMOVE: 'system:role:remove',
  GRANT: 'system:role:grant',
  USER_ROLE_ASSIGN: 'system:user-role:assign',
} as const;

export const CONFIG_PERMISSIONS = {
  QUERY: 'system:config:query',
  ADD: 'system:config:add',
  EDIT: 'system:config:edit',
  DELETE: 'system:config:delete',
  GROUP_QUERY: 'system:config:group:query',
  GROUP_ADD: 'system:config:group:add',
  GROUP_EDIT: 'system:config:group:edit',
  GROUP_DELETE: 'system:config:group:delete',
  SENSITIVE: 'system:config:sensitive',
  APPLY: 'system:config:apply',
  ROLLBACK: 'system:config:rollback',
  SCOPE_QUERY: 'system:config:scope:query',
  SCOPE_ASSIGN: 'system:config:scope:assign',
  CACHE_REFRESH: 'system:cache:refresh',
} as const;

export const DICT_PERMISSIONS = {
  QUERY: 'system:dict:query',
  ADD: 'system:dict:add',
  EDIT: 'system:dict:edit',
  DELETE: 'system:dict:delete',
} as const;

export const MENU_PERMISSIONS = {
  LIST: 'system:menu:list',
  QUERY: 'system:menu:query',
  ADD: 'system:menu:add',
  EDIT: 'system:menu:edit',
  REMOVE: 'system:menu:remove',
  PERMISSION_LIST: 'system:menu:permission:list',
  PERMISSION_ASSIGN: 'system:menu:permission:assign',
} as const;

export const PERMISSION_PERMISSIONS = {
  LIST: 'system:permission:list',
  QUERY: 'system:permission:query',
  ADD: 'system:permission:add',
  EDIT: 'system:permission:edit',
  REMOVE: 'system:permission:remove',
} as const;

export const SSO_CLIENT_PERMISSIONS = {
  LIST: 'system:sso-client:list',
  QUERY: 'system:sso-client:query',
  ADD: 'system:sso-client:add',
  EDIT: 'system:sso-client:edit',
  STATUS: 'system:sso-client:status',
  REDIRECT_LIST: 'system:sso-client:redirect:list',
  REDIRECT_EDIT: 'system:sso-client:redirect:edit',
  SECRET_LIST: 'system:sso-client:secret:list',
  SECRET_GENERATE: 'system:sso-client:secret:generate',
  SECRET_DISABLE: 'system:sso-client:secret:disable',
} as const;

export const EXTERNAL_LOGIN_PERMISSIONS = {
  PROVIDER_LIST: 'system:external-login-provider:list',
  PROVIDER_QUERY: 'system:external-login-provider:query',
  PROVIDER_ADD: 'system:external-login-provider:add',
  PROVIDER_EDIT: 'system:external-login-provider:edit',
  PROVIDER_STATUS: 'system:external-login-provider:status',
  PROVIDER_SECRET_ROTATE: 'system:external-login-provider:secret:rotate',
  IDENTITY_LIST: 'system:external-login-identity:list',
  IDENTITY_STATUS: 'system:external-login-identity:status',
  TOKEN_LIST: 'system:external-oauth-token:list',
  TOKEN_REVOKE: 'system:external-oauth-token:revoke',
} as const;

export const ENTERPRISE_IDENTITY_PERMISSIONS = {
  CONNECTION_LIST: 'system:enterprise-connection:list',
  CONNECTION_EDIT: 'system:enterprise-connection:edit',
  CONNECTION_STATUS: 'system:enterprise-connection:status',
  IDENTITY_LIST: 'system:enterprise-identity:list',
  IDENTITY_QUERY: 'system:enterprise-identity:query',
  LOGIN_ACCESS: 'system:enterprise-identity:login-access',
} as const;

export const PLATFORM_PERMISSIONS = {
  LIST: 'system:platform:list',
  QUERY: 'system:platform:query',
  ADD: 'system:platform:add',
  EDIT: 'system:platform:edit',
  STATUS: 'system:platform:status',
  LOGIN_METHODS: 'system:platform:login-method:edit',
  SOURCE_RULES: 'system:platform:source-rule:edit',
  DEFAULT_ROLES: 'system:platform:default-role:edit',
} as const;

export const HUB_NODE_PERMISSIONS = {
  LIST: 'system:hub-node:list',
  QUERY: 'system:hub-node:query',
  ADD: 'system:hub-node:add',
  EDIT: 'system:hub-node:edit',
  STATUS: 'system:hub-node:status',
  TEST: 'system:hub-node:test',
  USER_LIST: 'system:hub-node:user:list',
  USER_QUERY: 'system:hub-node:user:query',
  USER_STATUS: 'system:hub-node:user:status',
  SESSION_LIST: 'system:hub-node:session:list',
  SESSION_REVOKE: 'system:hub-node:session:revoke',
  POLICY_QUERY: 'system:hub-node:policy:query',
  POLICY_APPLY: 'system:hub-node:policy:apply',
  FEDERATION_QUERY: 'system:hub-node:federation:query',
  FEDERATION_APPLY: 'system:hub-node:federation:apply',
} as const;

export const NOTIFICATION_PERMISSIONS = {
  CHANNEL_LIST: 'system:notification:channel:list',
  CHANNEL_EDIT: 'system:notification:channel:edit',
  TEMPLATE_LIST: 'system:notification:template:list',
  TEMPLATE_EDIT: 'system:notification:template:edit',
  SCENE_LIST: 'system:notification:scene:list',
  SCENE_EDIT: 'system:notification:scene:edit',
  DELIVERY_LIST: 'system:notification:delivery:list',
  DELIVERY_DIAGNOSTIC: 'system:notification:delivery:diagnostic',
  DELIVERY_CONTENT_PUBLIC: 'system:notification:delivery:content:public',
  DELIVERY_CONTENT_SENSITIVE: 'system:notification:delivery:content:sensitive',
  DELIVERY_CONTENT_SECRET_EPHEMERAL: 'system:notification:delivery:content:secret-ephemeral',
  TEST: 'system:notification:test',
} as const;
