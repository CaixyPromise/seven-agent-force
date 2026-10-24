import { getSimpleUserById } from '@/api/userController';
import { useQuery } from '@tanstack/react-query';
import { Skeleton, Typography } from 'antd';

const { Text } = Typography;

export interface CreatorIdentityProps {
  userId?: API.Int64;
}

/**
 * Resolves a stored Workforce user ID through Framework's bounded UserSelector
 * facade. Task records remain stable when an account is renamed; callers never
 * treat a mutable username as ownership identity.
 */
export default function CreatorIdentity({ userId }: CreatorIdentityProps) {
  const normalized = String(userId ?? '').trim();
  const query = useQuery({
    queryKey: ['system', 'user', 'simple', normalized],
    queryFn: async () => (await getSimpleUserById({ id: normalized })).data,
    enabled: Boolean(normalized),
    staleTime: 60_000,
    retry: false,
  });

  if (!normalized) return <Text type="secondary">创建人未知</Text>;
  if (query.isLoading) return <Skeleton.Input active size="small" style={{ width: 88 }} />;
  if (query.data) {
    return <Text>{query.data.nickName || query.data.username || `用户 ${normalized}`}</Text>;
  }
  return <Text type="secondary">用户不可见</Text>;
}
