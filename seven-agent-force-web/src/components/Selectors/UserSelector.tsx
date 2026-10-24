import { useEffect, useRef, useState } from 'react';
import { Avatar, Empty, Select, Spin, Tag } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import { getUserOptions, searchUsers } from '@/api/userController';

type SelectorValue = API.Int64;

export interface UserOption {
  id: SelectorValue;
  userAccount: string;
  nickName: string;
  userAvatar?: string;
  status: number;
}

interface UserSelectorProps {
  value?: SelectorValue | SelectorValue[];
  onChange?: (value: SelectorValue | SelectorValue[], users?: UserOption | UserOption[]) => void;
  mode?: 'single' | 'multiple';
  placeholder?: string;
  disabled?: boolean;
  allowClear?: boolean;
  filterByStatus?: number;
  filterByDept?: API.Int64;
  excludeUsers?: SelectorValue[];
  dataSource?: UserOption[];
  onSearch?: (keyword: string) => Promise<UserOption[]>;
  maxTagCount?: number;
  style?: React.CSSProperties;
  className?: string;
}

const OPTION_LIMIT = 20;

export function UserSelector({
  value,
  onChange,
  mode = 'single',
  placeholder = '请选择用户',
  disabled = false,
  allowClear = true,
  filterByStatus = 0,
  filterByDept,
  excludeUsers = [],
  dataSource,
  onSearch,
  maxTagCount = 3,
  style,
  className,
}: UserSelectorProps) {
  const [users, setUsers] = useState<UserOption[]>(dataSource ?? []);
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const requestSequence = useRef(0);

  useEffect(() => {
    const sequence = ++requestSequence.current;
    if (dataSource) {
      return;
    }
    const timer = window.setTimeout(async () => {
      setLoading(true);
      try {
        if (onSearch) {
          const result = await onSearch(keyword);
          if (requestSequence.current === sequence) {
            setUsers(result);
          }
          return;
        }
        const response = keyword
          ? await searchUsers({ keyword, limit: OPTION_LIMIT, deptId: filterByDept })
          : await getUserOptions({ limit: OPTION_LIMIT, deptId: filterByDept });
        if (requestSequence.current !== sequence) return;
        setUsers(
          (response.data ?? [])
            .filter(
              (user): user is API.SimpleUserVO & { id: API.Int64 } =>
                typeof user.id === 'string' && user.id.length > 0,
            )
            .map((user) => ({
              id: user.id,
              userAccount: user.username ?? '',
              nickName: user.nickName || user.username || '',
              userAvatar: user.avatar,
              status: user.status ?? 0,
            })),
        );
      } catch (error) {
        if (requestSequence.current === sequence) {
          console.error('加载用户选项失败:', error);
          setUsers([]);
        }
      } finally {
        if (requestSequence.current === sequence) setLoading(false);
      }
    }, keyword ? 250 : 0);
    return () => {
      window.clearTimeout(timer);
      if (requestSequence.current === sequence) requestSequence.current += 1;
    };
  }, [dataSource, filterByDept, keyword, onSearch]);

  const visibleUsers = dataSource ?? users;
  const visibleLoading = !dataSource && loading;
  const excluded = new Set(excludeUsers.map(String));
  const options = visibleUsers
    .filter((user) => !excluded.has(String(user.id)))
    .filter((user) => filterByStatus === undefined || user.status === filterByStatus)
    .map((user) => ({ value: user.id, label: `${user.nickName} (${user.userAccount})`, user }));

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
      }}
      onChange={(nextValue) => {
        if (Array.isArray(nextValue)) {
          const selected = nextValue
            .map((id) => visibleUsers.find((user) => String(user.id) === String(id)))
            .filter((user): user is UserOption => Boolean(user));
          onChange?.(nextValue, selected);
        } else {
          onChange?.(nextValue, visibleUsers.find((user) => String(user.id) === String(nextValue)));
        }
      }}
      options={options}
      optionRender={(option) => {
        const user = option.data.user as UserOption;
        return (
          <div className="flex items-center gap-2">
            <Avatar size="small" src={user.userAvatar} icon={<UserOutlined />} />
            <span className="min-w-0 flex-1 truncate">{option.label}</span>
            <Tag color={user.status === 0 ? 'green' : 'red'}>{user.status === 0 ? '启用' : '停用'}</Tag>
          </div>
        );
      }}
      loading={visibleLoading}
      maxTagCount={mode === 'multiple' ? maxTagCount : undefined}
      notFoundContent={
        visibleLoading ? <Spin size="small" /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />
      }
      style={style}
      className={className}
    />
  );
}

export default UserSelector;
