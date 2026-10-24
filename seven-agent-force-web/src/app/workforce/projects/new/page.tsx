import { ArrowLeftOutlined } from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import { App, Button, Flex, Space, Typography } from 'antd';
import { useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { createProject } from '@/api/projectController';
import { projectFeedback, type ProjectDraft } from '@/api/projectContract';
import ProjectForm from '../_components/ProjectForm';
import styles from '../projects.module.css';

const { Text, Title } = Typography;

export default function NewProjectPage() {
  const navigate = useNavigate();
  const { message } = App.useApp();
  const idempotencyKey = useRef(`web-project:${crypto.randomUUID()}`);
  const projectKey = useRef(`project-${crypto.randomUUID()}`);
  const create = useMutation({
    mutationFn: (input: ProjectDraft) => createProject({ ...input, projectKey: input.projectKey || projectKey.current }, idempotencyKey.current),
    onSuccess: (project) => {
      create.reset();
      message.success('项目已创建');
      navigate(`/workforce/projects/${encodeURIComponent(project.id)}`, { replace: true });
    },
    onError: (error) => message.error(projectFeedback(error)),
  });

  return <main className={styles.page}>
    <Flex className={styles.editorHeader} align="center" gap={14}>
      <Button aria-label="返回项目列表" type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/workforce/projects')} />
      <Space orientation="vertical" size={1}>
        <Title level={2}>新建项目</Title>
        <Text type="secondary">配置一次，后续任务直接继承。</Text>
      </Space>
    </Flex>
    <ProjectForm submitting={create.isPending} onSubmit={(value) => create.mutateAsync(value)} onCancel={() => navigate('/workforce/projects')} />
  </main>;
}
