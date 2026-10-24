import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const policySource = await readFile(
  new URL('../src/lib/navigation/runtimeFeaturePolicy.ts', import.meta.url),
  'utf8',
);
const { outputText, diagnostics = [] } = ts.transpileModule(policySource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  reportDiagnostics: true,
});
assert.deepEqual(diagnostics, []);

const policy = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const {
  SAFE_RUNTIME_FEATURES,
  buildRuntimeRouteManifest,
  filterRoutesByRuntimeFeatures,
  findRuntimeRouteFeature,
  isRuntimeFeatureEnabled,
  normalizeRuntimeFeatures,
} = policy;

test('uses features.enabled as the authoritative capability source', () => {
  const normalized = normalizeRuntimeFeatures({
    features: { enabled: ['platform.control', 'federation.hub', 'platform.control'] },
    platform: {
      mode: 'hub',
      capabilities: {
        controlPlane: false,
        federatedHubLogin: false,
        nodeApi: true,
      },
    },
    docker: { enabled: true },
  });

  assert.deepEqual(normalized.features.enabled, ['platform.control', 'federation.hub']);
  assert.equal(isRuntimeFeatureEnabled(normalized, 'platform.control'), true);
  assert.equal(isRuntimeFeatureEnabled(normalized, 'federation.node'), false);
  assert.equal(normalized.docker.enabled, false);
});

test('converts the legacy capability contract only when features.enabled is absent', () => {
  const normalized = normalizeRuntimeFeatures({
    platform: {
      mode: 'node',
      capabilities: {
        controlPlane: true,
        federatedHubLogin: true,
        nodeApi: true,
      },
    },
    docker: { enabled: true },
  });

  assert.deepEqual(normalized.features.enabled, [
    'platform.control',
    'federation.hub',
    'federation.node',
    'docker.admin',
  ]);
});

test('rejects malformed features.enabled instead of trusting legacy fields', () => {
  assert.throws(() => normalizeRuntimeFeatures({
    features: { enabled: 'platform.control' },
    platform: {
      mode: 'hub',
      capabilities: {
        controlPlane: true,
        federatedHubLogin: true,
        nodeApi: true,
      },
    },
    docker: { enabled: true },
  }), /运行特性接口返回异常/);
});

test('declares optional route capabilities and keeps external login core', () => {
  assert.equal(findRuntimeRouteFeature('/system/platform')?.featureCode, 'platform.control');
  assert.equal(findRuntimeRouteFeature('/system/hub-node/abc')?.featureCode, 'federation.hub');
  assert.equal(findRuntimeRouteFeature('/system/docker/images')?.featureCode, 'docker.admin');
  assert.equal(findRuntimeRouteFeature('/system/external-login-provider')?.featureCode, null);
});

test('filters disabled auto routes before router creation and preserves core routes', () => {
  const routes = [{
    children: [{
      path: 'system',
      children: [
        { path: 'platform', element: 'platform' },
        { path: 'hub-node', element: 'hub' },
        { path: 'docker', element: 'docker' },
        { path: 'external-login-provider', element: 'external-login' },
        { path: 'user', element: 'user' },
      ],
    }],
  }];
  const features = normalizeRuntimeFeatures({
    features: { enabled: ['federation.hub'] },
  });

  const filtered = buildRuntimeRouteManifest(features, routes);
  const systemChildren = filtered[0].children[0].children;
  assert.deepEqual(systemChildren.map((route) => route.path), [
    'hub-node',
    'external-login-provider',
    'user',
  ]);
});

test('fails closed for feature-coded menu routes while leaving core routes available', () => {
  const routes = [
    { path: '/custom-node', name: 'Node', featureCode: 'federation.node' },
    { path: '/system/platform', name: 'Platform' },
    {
      path: '/system/external-login-provider',
      name: 'External login',
      featureCode: 'platform.control',
    },
    { path: '/system/user', name: 'User' },
  ];

  assert.deepEqual(
    filterRoutesByRuntimeFeatures(routes, SAFE_RUNTIME_FEATURES).map((route) => route.path),
    ['/system/external-login-provider', '/system/user'],
  );
});
