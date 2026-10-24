import { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Select, Spin, Tag, Typography } from 'antd';
import type { SelectProps } from 'antd';
import { getRolePage } from '@/api/sysRoleController';

type SelectorValue = string | number;

export interface RoleOption {
  id: SelectorValue;
  roleName: string;
  roleKey: string;
  dataScope: number;
  status: number;
  authorizationRoot: boolean;
  systemManaged: boolean;
  description?: string;
}

interface RoleSelectorProps {
  value?: SelectorValue | SelectorValue[];
  onChange?: (value: SelectorValue | SelectorValue[], roles?: RoleOption | RoleOption[]) => void;
  mode?: 'single' | 'multiple';
  placeholder?: string;
  disabled?: boolean;
  allowClear?: boolean;
  filterByStatus?: number;
  filterByDataScope?: number[];
  excludeRoles?: SelectorValue[];
  excludeAuthorizationRoot?: boolean;
  dataSource?: RoleOption[];
  onSearch?: (keyword: string) => Promise<RoleOption[]>;
  showStatus?: boolean;
  maxTagCount?: number;
  style?: React.CSSProperties;
  className?: string;
}

const PAGE_SIZE = 20;

export function RoleSelector({
  value,
  onChange,
  mode = 'single',
  placeholder = '请选择角色',
  disabled = false,
  allowClear = true,
  filterByStatus = 0,
  filterByDataScope,
  excludeRoles = [],
  excludeAuthorizationRoot = false,
  dataSource,
  onSearch,
  showStatus = false,
  maxTagCount = 3,
  style,
  className,
}: RoleSelectorProps) {
  const [roles, setRoles] = useState<RoleOption[]>(dataSource ?? []);
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(dataSource?.length ?? 0);
  const [loading, setLoading] = useState(false);
  const requestSequence = useRef(0);

  useEffect(() => {
    if (dataSource) {
      return;
    }
    const sequence = ++requestSequence.current;
    const timer = window.setTimeout(async () => {
      setLoading(true);
      try {
        if (onSearch) {
          const result = await onSearch(keyword);
          if (requestSequence.current === sequence) {
            setRoles(result);
            setTotal(result.length);
          }
          return;
        }
        const response = await getRolePage({
          current: page,
          size: PAGE_SIZE,
          name: keyword || undefined,
          status: filterByStatus,
        });
        if (requestSequence.current !== sequence) return;
        const records = response.data?.records ?? [];
        setRoles(
          records.map((role) => ({
            id: role.id ?? '',
            roleName: role.name ?? '',
            roleKey: role.code ?? '',
            dataScope: role.dataScope ?? 5,
            status: role.status ?? 1,
            authorizationRoot: role.authorizationRoot === true,
            systemManaged: role.systemManaged === true,
            description: role.remark,
          })),
        );
        setTotal(Number(response.data?.total ?? 0));
      } finally {
        if (requestSequence.current === sequence) setLoading(false);
      }
    }, keyword ? 250 : 0);
    return () => window.clearTimeout(timer);
  }, [dataSource, filterByStatus, keyword, onSearch, page]);

  const visibleRoles = dataSource ?? roles;
  const visibleTotal = dataSource?.length ?? total;
  const excluded = useMemo(() => new Set(excludeRoles.map(String)), [excludeRoles]);
  const options = visibleRoles
    .filter((role) => !excluded.has(String(role.id)))
    .filter((role) => filterByStatus === undefined || role.status === filterByStatus)
    .filter((role) => !filterByDataScope || filterByDataScope.includes(role.dataScope))
    .filter((role) => !excludeAuthorizationRoot || !role.authorizationRoot)
    .map((role) => ({
      value: role.id,
      label: `${role.roleName} (${role.roleKey})`,
      role,
    }));

  const handleChange: SelectProps<SelectorValue | SelectorValue[]>['onChange'] = (nextValue) => {
    if (Array.isArray(nextValue)) {
      const selected = nextValue
        .map((id) => visibleRoles.find((role) => String(role.id) === String(id)))
        .filter((role): role is RoleOption => Boolean(role));
      onChange?.(nextValue, selected);
      return;
    }
    onChange?.(nextValue, visibleRoles.find((role) => String(role.id) === String(nextValue)));
  };

  return (
    <Select<SelectorValue | SelectorValue[]>
      value={value}
      mode={mode === 'multiple' ? 'multiple' : undefined}
      placeholder={placeholder}
      disabled={disabled}
      allowClear={allowClear}
      showSearch
      filterOption={false}
      onSearch={(nextKeyword) => {
        setKeyword(nextKeyword.trim());
        setPage(1);
      }}
      onChange={handleChange}
      options={options}
      loading={loading}
      maxTagCount={mode === 'multiple' ? maxTagCount : undefined}
      notFoundContent={loading ? <Spin size="small" /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />}
      popupRender={(menu) => (
        <>
          {menu}
          {visibleTotal > PAGE_SIZE ? (
            <div className="flex items-center justify-between border-t border-gray-100 px-3 py-2">
              <Typography.Text type="secondary">共 {visibleTotal} 个角色</Typography.Text>
              <span>
                <a onClick={() => setPage((current) => Math.max(1, current - 1))}>上一页</a>
                <Tag className="mx-2">{page}</Tag>
                <a onClick={() => setPage((current) => Math.min(Math.ceil(visibleTotal / PAGE_SIZE), current + 1))}>下一页</a>
              </span>
            </div>
          ) : null}
          {excludeAuthorizationRoot ? (
            <div className="border-t border-gray-100 px-3 py-2 text-xs text-gray-500">安全根只能直接授予用户，不能通过岗位继承。</div>
          ) : null}
        </>
      )}
      optionRender={(option) => {
        const role = option.data.role as RoleOption;
        return (
          <div className="flex items-center justify-between gap-2">
            <span>{option.label}</span>
            {showStatus ? <Tag color={role.status === 0 ? 'green' : 'red'}>{role.status === 0 ? '启用' : '停用'}</Tag> : null}
          </div>
        );
      }}
      style={style}
      className={className}
    />
  );
}

export default RoleSelector;
