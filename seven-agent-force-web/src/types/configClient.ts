/**
 * 配置客户端类型定义
 */

/**
 * 配置值类型
 */
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

/**
 * 配置值对象（客户端接口使用）
 */
export interface ConfigValueDTO {
  /** 配置键 */
  key: string;
  /** 值类型：string/int/boolean/json/array */
  type: ConfigValueType;
  /** 已按声明类型解码的配置值 */
  value: unknown;
  /** 分组编码 */
  groupCode?: string;
  /** 分组名称 */
  groupName?: string;
  schemaVersion: number;
  version: API.Int64;
  /** 内部标记：未找到 */
  _notFound?: boolean;
  /** 内部标记：请求错误 */
  _error?: boolean;
}

/**
 * 配置批量查询请求
 */
export interface ConfigBatchRequest {
  /** 配置键列表 */
  configKeys: string[];
}

/**
 * 配置获取选项（支持 groupCode + key 方式）
 */
export interface ConfigOptions {
  /** 分组编码 */
  groupCode: string;
  /** 配置键 */
  key: string;
}

/**
 * 配置缓存状态
 */
export interface ConfigCacheState {
  /** 配置缓存 Map<configKey, ConfigValueDTO> */
  cache: Map<string, ConfigValueDTO>;
  /** 正在加载的配置键 */
  loading: Set<string>;
  /** 加载失败的配置键 */
  failed: Set<string>;
}

/**
 * 配置结果包装对象（支持泛型）
 * 提供解析后的值以及元数据信息
 */
export interface ConfigResult<T = string> {
  /** 解析后的值（根据 type 自动类型转换） */
  value: T;
  /** 原始字符串值 */
  rawValue: string;
  /** 配置键 */
  key: string;
  /** 值类型 */
  type: ConfigValueType;
  /** 分组编码 */
  groupCode?: string;
  /** 分组名称 */
  groupName?: string;
}

/**
 * 解析配置值为指定类型
 * @param dto 配置值对象
 * @returns 解析后的值
 */
export function parseConfigValue<T>(dto: ConfigValueDTO): T {
  const { type, value } = dto;
  switch (type) {
    case 'INTEGER':
      if (typeof value !== 'number' || !Number.isInteger(value)) throw new Error('配置整数 wire type 不匹配');
      break;
    case 'BOOLEAN':
      if (typeof value !== 'boolean') throw new Error('配置布尔 wire type 不匹配');
      break;
    case 'MULTI_ENUM':
      if (!Array.isArray(value) || !value.every(item => typeof item === 'string')) {
        throw new Error('配置多选枚举 wire type 不匹配');
      }
      break;
    case 'JSON':
      if (value === null || typeof value !== 'object') throw new Error('配置 JSON wire type 不匹配');
      break;
    case 'DECIMAL':
    case 'STRING':
    case 'TEXT':
    case 'ENUM':
    case 'DATE':
    case 'DATETIME':
    case 'DURATION':
    case 'COLOR':
    case 'IMAGE':
    case 'FILE':
      if (typeof value !== 'string') throw new Error(`配置 ${type} wire type 不匹配`);
      break;
    default: {
      const exhaustive: never = type;
      throw new Error(`不支持的配置类型: ${exhaustive}`);
    }
  }
  return value as T;
}

/**
 * 从 ConfigValueDTO 构建 ConfigResult 包装对象
 * @param dto 配置值对象
 * @returns ConfigResult 包装对象
 */
export function buildConfigResult<T = string>(dto: ConfigValueDTO): ConfigResult<T> {
  return {
    value: parseConfigValue<T>(dto),
    rawValue: typeof dto.value === 'string' ? dto.value : JSON.stringify(dto.value),
    key: dto.key,
    type: dto.type,
    groupCode: dto.groupCode,
    groupName: dto.groupName,
  };
}

/**
 * 检查 ConfigValueDTO 是否为有效的配置（非 _notFound/_error 标记）
 */
export function isValidConfigDTO(dto: ConfigValueDTO | null): dto is ConfigValueDTO {
  if (!dto) return false;
  return !dto._notFound && !dto._error;
}
