/**
 * 字典客户端Hook导出
 */
export { DictClientProvider } from './DictClientContext';
export { useDictClientContext } from './useDictClientContext';
export {
  useDictValue,
  useDictValueOnly,
  useDictOptions,
  useDictOptionsOnly,
  useDictLabel,
  useDictItem,
  useDictItemExt,
  useDictLoading
} from '../useDictValue';
export type {
  DictItemVO,
  DictOptions,
  DictBatchRequest,
  DictBatchResponse,
  DictResult
} from '@/types/dictClient';
export {
  getDictLabel,
  getDictItemByValue,
  parseDictItemExt,
  buildDictResult,
  toDictOptions
} from '@/types/dictClient';
