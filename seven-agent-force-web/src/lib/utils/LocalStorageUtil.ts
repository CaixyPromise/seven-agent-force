export const LocalStorageUtil = {
  setItem(key: string, value: string) {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(key, value);
  },
  getItem(key: string) {
    if (typeof window === 'undefined') return null;
    return window.localStorage.getItem(key);
  },
  removeItem(key: string) {
    if (typeof window === 'undefined') return;
    window.localStorage.removeItem(key);
  },
  clear() {
    if (typeof window === 'undefined') return;
    window.localStorage.clear();
  },
};
