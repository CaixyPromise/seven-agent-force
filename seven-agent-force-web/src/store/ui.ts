import { create } from 'zustand';

interface UIState {
  siderCollapsed: boolean;
  toggleSider: (collapsed?: boolean) => void;
  modalMap: Record<string, boolean>;
  setModalVisible: (key: string, visible: boolean) => void;
}

export const useUIStore = create<UIState>((set) => ({
  siderCollapsed: false,
  toggleSider: (collapsed) =>
    set((state) => ({
      siderCollapsed: collapsed ?? !state.siderCollapsed,
    })),
  modalMap: {},
  setModalVisible: (key, visible) =>
    set((state) => ({
      modalMap: { ...state.modalMap, [key]: visible },
    })),
}));
