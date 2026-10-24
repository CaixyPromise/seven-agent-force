import type { ProFieldValueEnumType } from '@ant-design/pro-components';

export const USER_STATUS_NORMAL = 0;
export const USER_STATUS_DISABLED = 1;
export const USER_STATUS_PENDING_REVIEW = 2;

export const USER_STATUS_OPTIONS = [
  { value: USER_STATUS_NORMAL, label: '正常', color: 'green', proStatus: 'Success' },
  { value: USER_STATUS_PENDING_REVIEW, label: '待审核', color: 'gold', proStatus: 'Warning' },
  { value: USER_STATUS_DISABLED, label: '禁用', color: 'red', proStatus: 'Error' },
] as const;

export function getUserStatusLabel(status?: number | null) {
  return USER_STATUS_OPTIONS.find((item) => item.value === status)?.label ?? '未知';
}

export function getUserStatusColor(status?: number | null) {
  return USER_STATUS_OPTIONS.find((item) => item.value === status)?.color ?? 'default';
}

export function buildUserStatusValueEnum(): Record<number, ProFieldValueEnumType> {
  return USER_STATUS_OPTIONS.reduce<Record<number, ProFieldValueEnumType>>((target, item) => {
    target[item.value] = {
      text: item.label,
      status: item.proStatus,
    };
    return target;
  }, {});
}
