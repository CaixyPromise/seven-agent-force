import { createContext, useContext } from 'react';
import type {
  CreateDictTypeRequest,
  DictItem,
  DictType,
} from '@/types/dict';

export interface DictContextType {
  types: DictType[];
  typeTotal: number;
  typePageNum: number;
  typePageSize: number;
  selectedType: DictType | null;
  loadingTypes: boolean;
  setSelectedType: (type: DictType | null) => void;
  fetchTypes: (params?: { pageNum?: number; pageSize?: number; keyword?: string }) => Promise<void>;
  setTypePageNum: (pageNum: number) => void;
  setTypePageSize: (pageSize: number) => void;
  handleCreateType: (values: CreateDictTypeRequest) => Promise<void>;
  handleUpdateType: <K extends keyof DictType>(
    id: API.Int64,
    field: K,
    value: DictType[K],
  ) => Promise<void>;
  handleDeleteType: (id: API.Int64) => Promise<void>;
  handleMoveType: (
    id: API.Int64,
    beforeId?: API.Int64 | null,
    afterId?: API.Int64 | null,
  ) => Promise<void>;
  items: DictItem[];
  itemTotal: number;
  itemPageNum: number;
  itemPageSize: number;
  itemSearchTerm: string;
  loadingItems: boolean;
  fetchItems: (
    typeId: API.Int64,
    params?: { pageNum?: number; pageSize?: number; keyword?: string },
  ) => Promise<void>;
  setItemSearchTerm: (term: string) => void;
  setItemPageNum: (pageNum: number) => void;
  setItemPageSize: (pageSize: number) => void;
  addTempItem: (item: DictItem) => void;
  removeTempItem: (id: API.Int64) => void;
  handleCreateItem: (item: DictItem) => Promise<void>;
  handleUpdateItem: (item: DictItem) => Promise<void>;
  handleDeleteItem: (id: API.Int64) => Promise<void>;
  handleMoveItem: (
    itemId: API.Int64,
    beforeId?: API.Int64 | null,
    afterId?: API.Int64 | null,
  ) => Promise<void>;
  searchTerm: string;
  setSearchTerm: (term: string) => void;
  groupedTypes: Record<string, DictType[]>;
}

export const DictContext = createContext<DictContextType | undefined>(undefined);

export function useDictContext(): DictContextType {
  const context = useContext(DictContext);
  if (!context) {
    throw new Error('useDictContext must be used within DictProvider');
  }
  return context;
}
