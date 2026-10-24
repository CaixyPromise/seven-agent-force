'use client';

import { Descriptions, Drawer, Empty, Tabs, Tag } from 'antd';
import type { DockerImageDetailView } from '@/api/dockerController';
import { DockerCodeBlock, DockerSurfaceCard, formatBytes } from '../../components/dockerConsole';

interface ImageDetailDrawerProps {
  open: boolean;
  detail: DockerImageDetailView | null;
  onClose: () => void;
}

function formatTime(timestamp?: number) {
  if (!timestamp) {
    return '-';
  }
  return new Date(timestamp * 1000).toLocaleString();
}

export function ImageDetailDrawer({ open, detail, onClose }: ImageDetailDrawerProps) {
  return (
    <Drawer
      open={open}
      size="large"
      title={detail ? `镜像详情 · ${detail.imageId}` : '镜像详情'}
      onClose={onClose}
      destroyOnHidden
    >
      {!detail ? (
        <Empty description="暂无镜像详情" />
      ) : (
        <div className="space-y-5">
          <Tabs
            items={[
              {
                key: 'overview',
                label: '镜像概览',
                children: (
                  <DockerSurfaceCard>
                    <Descriptions bordered column={2} size="small">
                      <Descriptions.Item label="镜像 ID" span={2}>
                        <div className="break-all">{detail.imageId}</div>
                      </Descriptions.Item>
                      <Descriptions.Item label="镜像标签" span={2}>
                        {detail.repoTags?.length
                          ? detail.repoTags.map((tag) => (
                              <Tag color="blue" key={tag} className="mb-2 max-w-full overflow-hidden text-ellipsis align-middle">
                                {tag}
                              </Tag>
                            ))
                          : '无'}
                      </Descriptions.Item>
                      <Descriptions.Item label="镜像摘要" span={2}>
                        {detail.repoDigests?.length
                          ? detail.repoDigests.map((digest) => (
                              <div className="mb-2 max-w-full" key={digest}>
                                <div className="max-w-full break-all rounded-md bg-sky-50 px-3 py-1.5 text-sky-700">
                                  {digest}
                                </div>
                              </div>
                            ))
                          : '无'}
                      </Descriptions.Item>
                      <Descriptions.Item label="镜像大小">{formatBytes(detail.size)}</Descriptions.Item>
                      <Descriptions.Item label="创建时间">{formatTime(detail.created)}</Descriptions.Item>
                      <Descriptions.Item label="引用容器数">{detail.usedByContainerCount ?? 0}</Descriptions.Item>
                      <Descriptions.Item label="标签数量">
                        {detail.labels ? Object.keys(detail.labels).length : 0}
                      </Descriptions.Item>
                      <Descriptions.Item label="镜像元标签" span={2}>
                        {detail.labels && Object.keys(detail.labels).length > 0
                          ? Object.entries(detail.labels).map(([key, value]) => (
                              <div className="mb-2 max-w-full" key={key}>
                                <div className="max-w-full break-all rounded-md bg-cyan-50 px-3 py-1.5 text-cyan-700">
                                  {`${key}=${value}`}
                                </div>
                              </div>
                            ))
                          : '无'}
                      </Descriptions.Item>
                    </Descriptions>
                  </DockerSurfaceCard>
                ),
              },
              {
                key: 'inspect',
                label: '原始 Inspect',
                children: (
                  <DockerCodeBlock
                    title="镜像 Inspect"
                    description="保留 Docker daemon 返回的完整镜像结构，方便比对摘要、标签或 Layer 元数据。"
                    value={JSON.stringify(detail.inspect, null, 2)}
                  />
                ),
              },
            ]}
          />
        </div>
      )}
    </Drawer>
  );
}
