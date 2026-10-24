/**
 * 配置客户端Hook导出
 */
export { ConfigClientProvider } from './ConfigClientContext';
export { useConfigClientContext } from './useConfigClientContext';
export {
  useConfigValue,
  useConfigValueOnly,
  useConfigDTO,
  useConfigLoading
} from '../useConfigValue';
export type {
  ConfigValueDTO,
  ConfigOptions,
  ConfigBatchRequest,
  ConfigResult,
  ConfigValueType
} from '@/types/configClient';
export {
  parseConfigValue,
  buildConfigResult,
  isValidConfigDTO
} from '@/types/configClient';
