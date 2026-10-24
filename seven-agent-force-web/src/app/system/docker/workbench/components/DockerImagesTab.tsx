'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { message } from 'antd';
import {
  getDockerRegistries,
  type DockerRemoteRegistryView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerEmptyState } from '../../components/dockerConsole';
import { LocalImageTable } from '../../images/components/LocalImageTable';

interface DockerImagesTabProps {
  refreshToken?: number;
}

export function DockerImagesTab({ refreshToken = 0 }: DockerImagesTabProps) {
  const [registries, setRegistries] = useState<DockerRemoteRegistryView[]>([]);
  const permissions = usePermissionFlags({
    canLocalImages: DOCKER_PERMISSIONS.IMAGE_LIST,
    canRemoteRegistries: DOCKER_PERMISSIONS.REGISTRY_LIST,
  });

  const loadRegistries = useCallback(async () => {
    if (!permissions.canRemoteRegistries) {
      return;
    }
    try {
      const response = await getDockerRegistries();
      setRegistries(response.data || []);
    } catch (error) {
      message.error((error as Error).message || '获取远程仓库配置失败');
    }
  }, [permissions.canRemoteRegistries]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadRegistries();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadRegistries, refreshToken]);

  const enabledRegistries = useMemo(
    () => registries.filter((registry) => registry.status !== 1),
    [registries],
  );

  return (
    <div>
      {permissions.canLocalImages ? (
        <LocalImageTable refreshToken={refreshToken} registries={enabledRegistries} />
      ) : (
        <DockerEmptyState
          title="无本地镜像权限"
          description="当前账号没有查看本地镜像的权限。镜像源管理请从 Docker / 镜像源进入。"
        />
      )}
    </div>
  );
}

export default DockerImagesTab;
