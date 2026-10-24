/**
 * 字典客户端类型定义
 */

/**
 * 字典项视图对象
 */
export interface DictItemVO {
  /** 主键ID */
  id: API.Int64;
  /** 字典类型ID */
  dictTypeId: API.Int64;
  /** 字典类型编码 */
  dictCode: string;
  /** 字典类型名称 */
  dictName: string;
  /** 字典值（业务使用） */
  itemValue: string;
  /** 字典显示值 */
  itemLabel: string;
  /** 字典项描述 */
  itemDesc?: string;
  /** 排序号（升序） */
  sortOrder: number;
  /** 状态：1启用 0禁用 */
  status: number;
  colorToken?: 'gray' | 'blue' | 'pink' | 'green' | 'orange' | 'red' | 'purple';
  iconToken?: 'unknown' | 'male' | 'female' | 'check' | 'close' | 'info';
  presentationVersion: number;
  version: API.Int64;
  /** 创建时间 */
  createTime?: string;
  /** 更新时间 */
  updateTime?: string;
}

/**
 * 批量获取字典请求
 */
export interface DictBatchRequest {
  /** 字典类型编码列表 */
  dictCodes: string[];
  /** 是否强制查询数据库（不走缓存） */
  force?: boolean;
}

/**
 * 批量获取字典响应
 */
export interface DictBatchResponse {
  /** 字典数据，key为dictCode，value为字典项列表 */
  record: Record<string, DictItemVO[]>;
  /** 不存在或已禁用的字典类型编码列表 */
  missing: string[];
}

/**
 * 字典获取选项（支持 id 方式）
 */
export interface DictOptions {
  /** 字典类型ID */
  id: API.Int64;
}

/**
 * 字典缓存状态
 */
export interface DictCacheState {
  /** 字典缓存 Map<dictCode, DictItemVO[]> */
  cache: Map<string, DictItemVO[]>;
  /** 正在加载的字典编码 */
  loading: Set<string>;
  /** 加载失败的字典编码 */
  failed: Set<string>;
}

/**
 * 字典结果包装对象（支持泛型）
 * 提供字典项列表以及元数据信息
 */
export interface DictResult<T = DictItemVO[]> {
  /** 字典编码 */
  dictCode: string;
  /** 字典项列表（原始数据） */
  items: DictItemVO[];
  /** 泛型值（默认为 items，可通过 transformer 转换为其他格式） */
  value: T;
}

/**
 * 从字典项数组构建 DictResult 包装对象
 * @param dictCode 字典编码
 * @param items 字典项数组
 * @param transformer 可选的转换函数，用于将 items 转换为自定义类型
 * @returns DictResult 包装对象
 */
export function buildDictResult<T = DictItemVO[]>(
  dictCode: string,
  items: DictItemVO[],
  transformer?: (items: DictItemVO[]) => T
): DictResult<T> {
  return {
    dictCode,
    items,
    value: transformer ? transformer(items) : (items as unknown as T),
  };
}

/**
 * 解析字典项扩展属性
 * @param item 字典项
 * @returns 解析后的扩展属性对象
 */
export function parseDictItemExt<T extends Record<string, unknown>>(item: DictItemVO): T | null {
  if (!item.colorToken && !item.iconToken) return null;
  return {
    color: item.colorToken,
    icon: item.iconToken,
  } as unknown as T;
}

/**
 * 根据value获取字典项
 * @param items 字典项列表
 * @param value 字典值
 * @returns 匹配的字典项
 */
export function getDictItemByValue(items: DictItemVO[], value: string): DictItemVO | undefined {
  return items.find(item => item.itemValue === value);
}

/**
 * 根据value获取字典项标签
 * @param items 字典项列表
 * @param value 字典值
 * @returns 字典项标签
 */
export function getDictLabel(items: DictItemVO[], value: string): string {
  const item = getDictItemByValue(items, value);
  return item?.itemLabel || value;
}

/**
 * 将字典项列表转换为下拉选项格式
 * @param items 字典项列表
 * @returns 选项数组
 */
export function toDictOptions(items: DictItemVO[]): { value: string; label: string }[] {
  return items.map(item => ({
    value: item.itemValue,
    label: item.itemLabel,
  }));
}
