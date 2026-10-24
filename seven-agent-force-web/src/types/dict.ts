/**
 * 字典管理相关类型定义
 */

/**
 * 字典类型实体
 */
export interface DictType {
  /** ID */
  id: API.Int64;
  /** 字典编码 (唯一) */
  dictCode: string;
  /** 字典名称 */
  dictName: string;
  /** 所属模块 */
  module: string;
  /** 状态 (1=启用, 0=禁用) */
  status: number;
  /** 字典描述 */
  dictDesc?: string;
  /** 是否系统内置 (1=是, 0=否) */
  isSystem: number;
  /** 创建时间 */
  createTime?: string;
  /** 更新时间 */
  updateTime?: string;
  /** 排序值 */
  sortOrder: number;
  /** 字典项数量 */
  itemCount?: number;
  valueType: 'STRING' | 'INTEGER' | 'DECIMAL' | 'BOOLEAN' | 'DATE' | 'DATETIME' | 'DURATION' | 'COLOR';
  uiWidget: 'SELECT';
  validation?: Record<string, unknown>;
  exposure: 'INTERNAL' | 'AUTHENTICATED' | 'PUBLIC';
  sensitivity: 'NORMAL' | 'SENSITIVE' | 'SECRET';
  schemaVersion: number;
  version: API.Int64;
}

/**
 * 字典项实体
 */
export interface DictItem {
  /** ID */
  id: API.Int64;
  /** 字典类型ID */
  dictTypeId: API.Int64;
  /** 字典项值 (实际存储的值) */
  itemValue: string;
  /** 字典项标签 (显示的文本) */
  itemLabel: string;
  /** 状态 (1=启用, 0=禁用) */
  status: number;
  /** 排序值 */
  sortOrder: number;
  colorToken?: 'gray' | 'blue' | 'pink' | 'green' | 'orange' | 'red' | 'purple';
  iconToken?: 'unknown' | 'male' | 'female' | 'check' | 'close' | 'info';
  presentationVersion: number;
  version: API.Int64;
  /** 字典项描述 */
  itemDesc?: string;
  /** 创建时间 */
  createTime?: string;
  /** 更新时间 */
  updateTime?: string;
}

/**
 * 创建字典类型请求
 */
export interface CreateDictTypeRequest {
  /** 字典编码 */
  dictCode: string;
  /** 字典名称 */
  dictName: string;
  /** 所属模块 */
  module: string;
  /** 字典描述 */
  dictDesc?: string;
  /** 是否系统内置 */
  isSystem?: boolean;
  valueType?: DictType['valueType'];
  uiWidget?: 'SELECT';
  validation?: Record<string, unknown>;
  exposure?: DictType['exposure'];
  sensitivity?: DictType['sensitivity'];
  schemaVersion?: number;
}

/**
 * 更新字典类型请求
 */
export interface UpdateDictTypeRequest {
  /** ID */
  id: API.Int64;
  /** 字典编码 */
  dictCode?: string;
  /** 字典名称 */
  dictName?: string;
  /** 所属模块 */
  module?: string;
  /** 字典描述 */
  dictDesc?: string;
  /** 是否系统内置 */
  isSystem?: number;
  /** 排序值 */
  sortOrder?: number;
  valueType?: DictType['valueType'];
  uiWidget?: 'SELECT';
  validation?: Record<string, unknown>;
  exposure?: DictType['exposure'];
  sensitivity?: DictType['sensitivity'];
  schemaVersion?: number;
  version?: API.Int64;
}

/**
 * 字典类型查询参数
 */
export interface DictTypeQuery {
  /** 当前页 */
  pageNum?: number;
  /** 每页大小 */
  pageSize?: number;
  /** 关键词 */
  keyword?: string;
  /** 字典编码 */
  dictCode?: string;
  /** 字典名称 */
  dictName?: string;
  /** 所属模块 */
  module?: string;
  /** 状态 */
  status?: number;
}

/**
 * 创建字典项请求
 */
export interface CreateDictItemRequest {
  /** 字典类型ID */
  dictTypeId: API.Int64;
  /** 字典项值 */
  itemValue: string;
  /** 字典项标签 */
  itemLabel: string;
  /** 排序值 */
  sortOrder?: number;
  colorToken?: DictItem['colorToken'];
  iconToken?: DictItem['iconToken'];
  /** 字典项描述 */
  itemDesc?: string;
}

/**
 * 更新字典项请求
 */
export interface UpdateDictItemRequest {
  /** ID */
  id: API.Int64;
  /** 字典项标签 */
  itemLabel?: string;
  /** 字典项描述 */
  itemDesc?: string;
  /** 排序值 */
  sortOrder?: number;
  /** 状态 */
  status?: number;
  colorToken?: DictItem['colorToken'];
  iconToken?: DictItem['iconToken'];
  version?: API.Int64;
}

/**
 * 批量更新排序请求
 */
export interface BatchUpdateSortRequest {
  /** 字典类型ID */
  typeId: API.Int64;
  /** ID与排序值的映射 */
  items: Array<{
    id: API.Int64;
    sortOrder: number;
  }>;
}

/**
 * 移动排序请求
 */
export interface MoveSortRequest {
  /** 字典类型ID */
  typeId?: API.Int64;
  /** 前一个元素ID */
  beforeId?: API.Int64 | null;
  /** 后一个元素ID */
  afterId?: API.Int64 | null;
}

/**
 * 字典项查询参数
 */
export interface DictItemQuery {
  /** 字典类型ID */
  dictTypeId: API.Int64;
  /** 状态 */
  status?: number;
  /** 关键词 */
  keyword?: string;
}
