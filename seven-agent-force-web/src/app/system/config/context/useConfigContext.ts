import { createContext, useContext } from 'react';
import type {
  ConfigGroup,
  ConfigItem,
  CreateConfigGroupRequest,
} from '@/types/config';

export interface ConfigContextType {
  groups: ConfigGroup[];
  groupTotal: number;
  groupPageNum: number;
  groupPageSize: number;
  activeGroup: ConfigGroup | null;
  loadingGroups: boolean;
  setActiveGroup: (group: ConfigGroup | null) => void;
  fetchGroups: (params?: { pageNum?: number; pageSize?: number; keyword?: string }) => Promise<void>;
  setGroupPageNum: (pageNum: number) => void;
  setGroupPageSize: (pageSize: number) => void;
  handleCreateGroup: (values: CreateConfigGroupRequest) => Promise<void>;
  handleUpdateGroup: (id: API.Int64, field: keyof ConfigGroup, value: unknown) => Promise<void>;
  handleDeleteGroup: (id: API.Int64) => Promise<void>;
  handleMoveGroup: (
    id: API.Int64,
    beforeId?: API.Int64 | null,
    afterId?: API.Int64 | null,
  ) => Promise<void>;
  configs: ConfigItem[];
  configTotal: number;
  configPageNum: number;
  configPageSize: number;
  loadingConfigs: boolean;
  fetchConfigs: (params?: {
    groupId?: API.Int64;
    pageNum?: number;
    pageSize?: number;
    autoFallbackWhenEmpty?: boolean;
    forceKeyword?: string;
    forceSearchType?: 'label' | 'key' | 'both';
  }) => Promise<void>;
  addTempConfig: (config: ConfigItem) => void;
  removeTempConfig: (id: API.Int64) => void;
  handleCreateConfig: (config: ConfigItem & { assetFileId?: API.Int64 }) => Promise<void>;
  handleUpdateConfig: (config: Partial<ConfigItem> & Pick<ConfigItem, 'id'> & {
    assetFileId?: API.Int64;
    clearAsset?: boolean;
  }) => Promise<void>;
  handleDeleteConfig: (id: API.Int64) => Promise<void>;
  searchTerm: string;
  setSearchTerm: (term: string) => void;
  groupedGroups: Record<string, ConfigGroup[]>;
  configSearchText: string;
  setConfigSearchText: (text: string) => void;
  configSearchType: 'label' | 'key' | 'both';
  setConfigSearchType: (searchType: 'label' | 'key' | 'both') => void;
  setConfigPageNum: (pageNum: number) => void;
  setConfigPageSize: (pageSize: number) => void;
}

export const ConfigContext = createContext<ConfigContextType | undefined>(undefined);

export function useConfigContext(): ConfigContextType {
  const context = useContext(ConfigContext);
  if (!context) {
    throw new Error('useConfigContext must be used within ConfigProvider');
  }
  return context;
}
