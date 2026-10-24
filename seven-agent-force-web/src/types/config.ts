/**
 * 配置管理相关类型定义
 */

/**
 * 配置分组实体
 */
export interface ConfigGroup {
  /** ID */
  id: API.Int64;
  /** 分组编码 (唯一) */
  groupCode: string;
  /** 分组名称 */
  groupName: string;
  /** 所属模块 */
  module: string;
  /** 排序值 */
  sortOrder: number;
  /** 创建时间 */
  createTime?: string;
  /** 更新时间 */
  updateTime?: string;
  /** 配置项数量 */
  configCount?: number;
  /** 当前用户对该分组的操作范围 */
  access?: ConfigAccess;
}

/**
 * 配置项实体
 */
export interface ConfigItem {
  /** ID */
  id: API.Int64;
  /** 分组ID */
  groupId: API.Int64;
  /** 配置键 */
  configKey: string;
  /** 配置值 */
  configValue: string;
  /** 值类型 */
  valueType: ConfigValueType;
  /** 配置描述 */
  configDesc?: string;
  /** 是否敏感 (1=是, 0=否) */
  isSensitive: number;
  /** 是否只读 (1=是, 0=否) */
  isReadonly: number;
  /** 是否启用 (1=是, 0=否) */
  isEnabled: number;
  /** 生效方式 (realtime=即时, restart=重启) */
  effectType: 'realtime' | 'restart';
  uiWidget: ConfigUIWidget;
  validation?: ScalarValidation;
  exposure: ConfigExposure;
  sensitivity: ConfigSensitivity;
  schemaVersion: number;
  version: API.Int64;
  valuePresent: boolean;
  connected: boolean;
  consumerStatus: 'CONNECTED' | 'UNCONNECTED';
  /** 排序值 */
  sortOrder?: number;
  /** 创建时间 */
  createTime?: string;
  /** 更新时间 */
  updateTime?: string;
  /** 当前用户对该配置项的操作范围 */
  access?: ConfigAccess;
}

export type ConfigValueType =
  | 'STRING'
  | 'TEXT'
  | 'INTEGER'
  | 'DECIMAL'
  | 'BOOLEAN'
  | 'ENUM'
  | 'MULTI_ENUM'
  | 'DATE'
  | 'DATETIME'
  | 'DURATION'
  | 'COLOR'
  | 'JSON'
  | 'IMAGE'
  | 'FILE';

export type ConfigUIWidget =
  | 'INPUT'
  | 'TEXTAREA'
  | 'INPUT_NUMBER'
  | 'SWITCH'
  | 'SELECT'
  | 'MULTI_SELECT'
  | 'DATE_PICKER'
  | 'DATETIME_PICKER'
  | 'DURATION_INPUT'
  | 'COLOR_PICKER'
  | 'CONTROLLED_JSON'
  | 'IMAGE_UPLOAD'
  | 'FILE_UPLOAD';

export type ConfigExposure = 'INTERNAL' | 'AUTHENTICATED' | 'PUBLIC';
export type ConfigSensitivity = 'NORMAL' | 'SENSITIVE' | 'SECRET';

export interface ScalarValidation {
  required?: boolean;
  minLength?: number;
  maxLength?: number;
  minValue?: number;
  maxValue?: number;
  options?: string[];
  maxItems?: number;
}

/**
 * 配置范围授权结果
 */
export interface ConfigAccess {
  /** 可读 */
  canRead?: boolean;
  /** 可写 */
  canWrite?: boolean;
  /** 可删 */
  canDelete?: boolean;
  /** 授权来源：admin/group/key/none */
  accessSource?: 'admin' | 'group' | 'key' | 'none' | string;
}

/**
 * 角色配置范围授权
 */
export interface ConfigScopeGrant {
  /** 分组编码 */
  groupCode: string;
  /** 配置键；为空表示整个分组 */
  configKey?: string;
  /** 可读 */
  canRead?: number;
  /** 可写 */
  canWrite?: number;
  /** 可删 */
  canDelete?: number;
}

/**
 * 创建配置分组请求
 */
export interface CreateConfigGroupRequest {
  /** 分组编码 */
  groupCode: string;
  /** 分组名称 */
  groupName: string;
  /** 所属模块 */
  module: string;
}

/**
 * 更新配置分组请求
 */
export interface UpdateConfigGroupRequest {
  /** ID */
  id: API.Int64;
  /** 分组编码 */
  groupCode?: string;
  /** 分组名称 */
  groupName?: string;
  /** 所属模块 */
  module?: string;
  /** 排序值 */
  sortOrder?: number;
}

/**
 * 配置分组查询参数
 */
export interface ConfigGroupQuery {
  /** 当前页 */
  pageNum?: number;
  /** 每页大小 */
  pageSize?: number;
  /** 关键词 */
  keyword?: string;
  /** 分组编码 */
  groupCode?: string;
  /** 分组名称 */
  groupName?: string;
  /** 所属模块 */
  module?: string;
}

/**
 * 创建配置项请求
 */
export interface CreateConfigRequest {
  /** 分组ID */
  groupId: API.Int64;
  /** 配置键 */
  configKey: string;
  /** 配置值 */
  configValue?: string;
  /** 已上传但尚未绑定的文件；只在 IMAGE/FILE 保存时传递。 */
  assetFileId?: API.Int64;
  /** 值类型 */
  valueType: ConfigValueType;
  /** 配置描述 */
  configDesc?: string;
  /** 是否敏感 */
  isSensitive?: number;
  /** 是否只读 */
  isReadonly?: number;
  /** 生效方式 */
  effectType?: 'realtime' | 'restart';
  uiWidget?: ConfigUIWidget;
  validation?: ScalarValidation;
  exposure?: ConfigExposure;
  sensitivity?: ConfigSensitivity;
  schemaVersion?: number;
}

/**
 * 更新配置项请求
 */
export interface UpdateConfigRequest {
  /** ID */
  id: API.Int64;
  /** 配置键 */
  configKey?: string;
  /** 配置值 */
  configValue?: string;
  /** 已上传但尚未绑定的替换文件；只在 IMAGE/FILE 保存时传递。 */
  assetFileId?: API.Int64;
  /** 明确清除当前 IMAGE/FILE 绑定，不接受任何 URL 或 reference 数据。 */
  clearAsset?: boolean;
  /** 值类型 */
  valueType?: ConfigValueType;
  /** 配置描述 */
  configDesc?: string;
  /** 是否敏感 */
  isSensitive?: number;
  /** 是否只读 */
  isReadonly?: number;
  /** 是否启用 */
  isEnabled?: number;
  /** 生效方式 */
  effectType?: 'realtime' | 'restart';
  /** 排序值 */
  sortOrder?: number;
  uiWidget?: ConfigUIWidget;
  validation?: ScalarValidation;
  exposure?: ConfigExposure;
  sensitivity?: ConfigSensitivity;
  schemaVersion?: number;
  version?: API.Int64;
}

/**
 * 配置项查询参数
 */
export interface ConfigQuery {
  /** 分组ID */
  groupId: API.Int64;
  /** 当前页 */
  pageNum?: number;
  /** 每页大小 */
  pageSize?: number;
  /** 关键词（兼容后端旧参数） */
  keyword?: string;
  /** 搜索文本 */
  searchText?: string;
  /** 搜索维度 */
  searchType?: 'label' | 'key' | 'both';
  /** 配置键 */
  configKey?: string;
  /** 是否启用 */
  isEnabled?: number;
}

/**
 * 配置项分页结果
 */
export interface ConfigPageResult {
  records: ConfigItem[];
  total: number;
  pageNum: number;
  pageSize: number;
  pages: number;
}

/**
 * 敏感配置回显响应
 */
export interface ConfigSensitiveRevealResponse {
  encryptedValue: string;
}

/**
 * 批量更新排序请求
 */
export interface BatchUpdateGroupSortRequest {
  /** ID与排序值的映射 */
  groups: Array<{
    id: API.Int64;
    sortOrder: number;
  }>;
}

/**
 * 配置变更日志视图对象（审计合规）
 */
export interface ConfigChangeLog {
  /** 主键ID */
  id: API.Int64;
  /** 配置ID */
  configId: API.Int64;
  /** 配置键 */
  configKey: string;
  /** 操作类型：CREATE/UPDATE/DELETE/APPLY/ROLLBACK */
  operationType: 'CREATE' | 'UPDATE' | 'DELETE' | 'APPLY' | 'ROLLBACK';
  /** 旧值 */
  oldValue?: string;
  /** 新值 */
  newValue: string;
  /** 生效方式：realtime/restart */
  effectType: 'realtime' | 'restart';
  /** 状态：pending/applied/rolled_back */
  status: 'pending' | 'applied' | 'rolled_back';
  /** 父级日志ID（用于构建操作链，如ROLLBACK指向被回滚的UPDATE记录） */
  parentLogId?: API.Int64;
  /** 关联日志ID（用于更灵活的关联，如批量操作或操作链） */
  relatedLogId?: API.Int64;
  /** 操作人ID */
  operatorId?: API.Int64;
  /** 操作人姓名 */
  operatorName?: string;
  /** 操作时间 */
  operationTime?: string;
  /** 操作原因/说明 */
  operationReason?: string;
  /** 应用人ID（仅APPLY操作有值） */
  appliedBy?: API.Int64;
  /** 应用时间（仅APPLY操作有值） */
  appliedTime?: string;
}

/**
 * 待生效配置视图对象
 */
export interface PendingConfig {
  /** 变更日志ID */
  logId: API.Int64;
  /** 配置ID */
  configId: API.Int64;
  /** 配置键 */
  configKey: string;
  /** 配置描述 */
  configDesc?: string;
  /** 当前生效值 */
  currentValue: string;
  /** 待生效值 */
  pendingValue: string;
  /** 创建人ID */
  createdBy?: API.Int64;
  /** 创建人姓名 */
  createdByName?: string;
  /** 创建时间 */
  createTime?: string;
}

/**
 * 回滚配置变更请求
 */
export interface RollbackConfigRequest {
  /** 变更日志ID */
  logId: API.Int64;
  /** 回滚原因 */
  reason?: string;
}
