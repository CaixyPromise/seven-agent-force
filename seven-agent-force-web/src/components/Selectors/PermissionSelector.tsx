import { useEffect, useRef, useState } from 'react';
import { Empty, Select, Spin, Tag, Typography } from 'antd';
import { listPermissions } from '@/api/sysMenuController';

type SelectorValue = string | number;

export interface PermissionOption {
  id: SelectorValue;
  code: string;
  name: string;
  method?: string;
  path?: string;
}

interface PermissionSelectorProps {
  value?: SelectorValue;
  onChange?: (value?: SelectorValue, permission?: PermissionOption) => void;
  placeholder?: string;
  disabled?: boolean;
  allowClear?: boolean;
  style?: React.CSSProperties;
  className?: string;
}

const PAGE_SIZE = 20;

export function PermissionSelector({
  value,
  onChange,
  placeholder = '请选择权限',
  disabled = false,
  allowClear = true,
  style,
  className,
}: PermissionSelectorProps) {
  const [permissions, setPermissions] = useState<PermissionOption[]>([]);
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const requestSequence = useRef(0);

  useEffect(() => {
    const sequence = ++requestSequence.current;
    const timer = window.setTimeout(async () => {
      setLoading(true);
      try {
        const response = await listPermissions({
          current: page,
          size: PAGE_SIZE,
          code: keyword || undefined,
          status: 0,
        });
        if (requestSequence.current !== sequence) return;
        const data = response.data as unknown as
          | { records?: API.PermissionVO[]; total?: number }
          | API.PermissionVO[]
          | undefined;
        const records = Array.isArray(data) ? data : data?.records ?? [];
        setPermissions(
          records
            .filter((item) => item.id !== undefined && item.code)
            .map((item) => ({
              id: item.id!,
              code: item.code!,
              name: item.name || item.code!,
              method: item.method,
              path: item.path,
            })),
        );
        setTotal(Array.isArray(data) ? data.length : Number(data?.total ?? 0));
      } finally {
        if (requestSequence.current === sequence) setLoading(false);
      }
    }, keyword ? 250 : 0);
    return () => window.clearTimeout(timer);
  }, [keyword, page]);

  return (
    <Select<SelectorValue>
      value={value}
      placeholder={placeholder}
      disabled={disabled}
      allowClear={allowClear}
      showSearch
      filterOption={false}
      onSearch={(nextKeyword) => {
        setKeyword(nextKeyword.trim());
        setPage(1);
      }}
      onChange={(nextValue) => {
        onChange?.(
          nextValue,
          permissions.find((item) => String(item.id) === String(nextValue)),
        );
      }}
      options={permissions.map((permission) => ({
        value: permission.id,
        label: `${permission.name} (${permission.code})`,
        permission,
      }))}
      optionRender={(option) => {
        const permission = option.data.permission as PermissionOption;
        return (
          <div className="flex items-center justify-between gap-2">
            <span className="min-w-0 flex-1 truncate">{option.label}</span>
            <Tag>{permission.method || '权限'}</Tag>
          </div>
        );
      }}
      loading={loading}
      notFoundContent={loading ? <Spin size="small" /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />}
      popupRender={(menu) => (
        <>
          {menu}
          {total > PAGE_SIZE ? (
            <div className="flex items-center justify-between border-t border-gray-100 px-3 py-2">
              <Typography.Text type="secondary">共 {total} 项权限</Typography.Text>
              <span className="flex items-center gap-2">
                <button type="button" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>上一页</button>
                <Tag>{page}</Tag>
                <button type="button" disabled={page >= Math.ceil(total / PAGE_SIZE)} onClick={() => setPage((current) => Math.min(Math.ceil(total / PAGE_SIZE), current + 1))}>下一页</button>
              </span>
            </div>
          ) : null}
        </>
      )}
      style={style}
      className={className}
    />
  );
}

export default PermissionSelector;
