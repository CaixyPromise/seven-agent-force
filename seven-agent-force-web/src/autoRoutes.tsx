import { Suspense, createElement, lazy, type ComponentType, type ReactElement } from 'react';
import { Outlet, type RouteObject } from 'react-router-dom';
import { Spin } from 'antd';

type ModuleWithDefault = {
  default: ComponentType;
};

type ModuleLoader = () => Promise<ModuleWithDefault>;

interface TreeNode {
  segment: string;
  children: Map<string, TreeNode>;
  pageLoader?: ModuleLoader;
  layoutLoader?: ModuleLoader;
}

const pageModules = import.meta.glob<ModuleWithDefault>('./app/**/page.{ts,tsx}');
const layoutModules = import.meta.glob<ModuleWithDefault>('./app/**/layout.{ts,tsx}');

const routeFallback = (
  <div
    style={{
      minHeight: '50vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    }}
  >
    <Spin size="large" />
  </div>
);

function withRouteSuspense(element: ReactElement) {
  return <Suspense fallback={routeFallback}>{element}</Suspense>;
}

function createNode(segment: string): TreeNode {
  return {
    segment,
    children: new Map<string, TreeNode>(),
  };
}

function normalizeModuleDirectory(moduleKey: string, kind: 'page' | 'layout'): string {
  const relativePath = moduleKey.replace('./app/', '');
  const suffixPattern = kind === 'page' ? /\/?page\.(tsx|ts)$/ : /\/?layout\.(tsx|ts)$/;
  return relativePath.replace(suffixPattern, '');
}

function upsertNode(root: TreeNode, relativeDirectory: string): TreeNode {
  const segments = relativeDirectory ? relativeDirectory.split('/') : [];
  let current = root;

  for (const segment of segments) {
    let child = current.children.get(segment);
    if (!child) {
      child = createNode(segment);
      current.children.set(segment, child);
    }
    current = child;
  }

  return current;
}

function segmentToRoutePath(segment: string): string | undefined {
  if (!segment) return undefined;
  if (/^\([^)]+\)$/.test(segment)) return undefined;

  const catchAllMatch = segment.match(/^\[\.\.\.([^\]]+)\]$/);
  if (catchAllMatch) {
    return '*';
  }

  const dynamicMatch = segment.match(/^\[([^\]]+)\]$/);
  if (dynamicMatch) {
    return `:${dynamicMatch[1]}`;
  }

  return segment;
}

function sortedChildren(node: TreeNode): TreeNode[] {
  return [...node.children.values()].sort((a, b) => a.segment.localeCompare(b.segment));
}

function createLazyPageElement(loader: ModuleLoader) {
  const LazyPage = lazy(loader);
  return withRouteSuspense(createElement(LazyPage));
}

function createLazyLayoutElement(loader: ModuleLoader) {
  const LazyLayout = lazy(loader);
  return withRouteSuspense(createElement(LazyLayout, undefined, createElement(Outlet)));
}

function buildNodeRoute(node: TreeNode, isRoot = false): RouteObject {
  const route: RouteObject = {
    element: node.layoutLoader ? createLazyLayoutElement(node.layoutLoader) : createElement(Outlet),
  };

  if (!isRoot) {
    const path = segmentToRoutePath(node.segment);
    if (path) {
      route.path = path;
    }
  }

  const children: RouteObject[] = [];

  if (node.pageLoader) {
    children.push({ index: true, element: createLazyPageElement(node.pageLoader) });
  }

  for (const child of sortedChildren(node)) {
    children.push(buildNodeRoute(child));
  }

  if (children.length > 0) {
    route.children = children;
  }

  return route;
}

const routeTreeRoot = createNode('');

for (const [moduleKey, loader] of Object.entries(layoutModules)) {
  const targetNode = upsertNode(routeTreeRoot, normalizeModuleDirectory(moduleKey, 'layout'));
  targetNode.layoutLoader = loader;
}

for (const [moduleKey, loader] of Object.entries(pageModules)) {
  const targetNode = upsertNode(routeTreeRoot, normalizeModuleDirectory(moduleKey, 'page'));
  targetNode.pageLoader = loader;
}

export function buildAutoRoutes(): RouteObject[] {
  return [buildNodeRoute(routeTreeRoot, true)];
}

export const autoRoutes = buildAutoRoutes();
