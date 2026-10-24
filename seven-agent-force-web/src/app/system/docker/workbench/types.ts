export const DOCKER_WORKBENCH_TABS = [
  'overview',
  'containers',
  'compose',
  'images',
  'networks',
  'volumes',
  'registries',
  'config',
  'operations',
] as const;

export type DockerWorkbenchTabKey = (typeof DOCKER_WORKBENCH_TABS)[number];

export function isDockerWorkbenchTabKey(value: string | null | undefined): value is DockerWorkbenchTabKey {
  return DOCKER_WORKBENCH_TABS.includes(value as DockerWorkbenchTabKey);
}
