'use client';

import type {
  PasskeyAssertionPayload,
  PasskeyRegistrationPayload,
} from './webauthn';

export type PasskeyBridgeMode = 'assertion' | 'registration';

interface PasskeyBridgeRequest {
  mode: PasskeyBridgeMode;
  hints?: Record<string, unknown>;
  userName?: string;
  displayName?: string;
}

type PasskeyBridgeResult = PasskeyAssertionPayload | PasskeyRegistrationPayload;

const PASSKEY_BRIDGE_READY = 'seven-passkey-bridge:ready';
const PASSKEY_BRIDGE_REQUEST = 'seven-passkey-bridge:request';
const PASSKEY_BRIDGE_RESULT = 'seven-passkey-bridge:result';
const PASSKEY_BRIDGE_ERROR = 'seven-passkey-bridge:error';

function resolveBridgeUrl() {
  const currentUrl = new URL(window.location.href);
  currentUrl.hostname = 'localhost';
  currentUrl.pathname = '/passkey-bridge';
  currentUrl.searchParams.set('origin', window.location.origin);
  return currentUrl;
}

export function isPasskeyBridgeMessage(data: unknown, expectedType: string) {
  return (
    typeof data === 'object'
    && data !== null
    && (data as { type?: string }).type === expectedType
  );
}

export async function runPasskeyInLocalhostBridge(
  request: PasskeyBridgeRequest,
): Promise<PasskeyBridgeResult> {
  if (typeof window === 'undefined') {
    throw new Error('当前环境不支持 Passkey 桥接');
  }

  const bridgeUrl = resolveBridgeUrl();
  const bridgeOrigin = bridgeUrl.origin;
  const popup = window.open(
    bridgeUrl.toString(),
    'seven-passkey-bridge',
    'popup=yes,width=520,height=720,resizable=yes,scrollbars=yes',
  );

  if (!popup) {
    throw new Error('浏览器拦截了 Passkey 验证窗口，请允许弹窗后重试');
  }

  return new Promise<PasskeyBridgeResult>((resolve, reject) => {
    let settled = false;
    let requestPosted = false;

    const cleanup = () => {
      if (settled) {
        return;
      }
      settled = true;
      window.removeEventListener('message', handleMessage);
      window.clearInterval(closeWatcher);
      window.clearTimeout(timeoutId);
    };

    const fail = (message: string) => {
      cleanup();
      reject(new Error(message));
    };

    const succeed = (payload: PasskeyBridgeResult) => {
      cleanup();
      resolve(payload);
    };

    const handleMessage = (event: MessageEvent<unknown>) => {
      if (event.origin !== bridgeOrigin) {
        return;
      }

      if (isPasskeyBridgeMessage(event.data, PASSKEY_BRIDGE_READY)) {
        if (requestPosted) {
          return;
        }
        requestPosted = true;
        popup.postMessage(
          {
            type: PASSKEY_BRIDGE_REQUEST,
            payload: request,
          },
          bridgeOrigin,
        );
        return;
      }

      if (isPasskeyBridgeMessage(event.data, PASSKEY_BRIDGE_RESULT)) {
        succeed((event.data as { payload: PasskeyBridgeResult }).payload);
        popup.close();
        return;
      }

      if (isPasskeyBridgeMessage(event.data, PASSKEY_BRIDGE_ERROR)) {
        const message = (event.data as { message?: string }).message || 'Passkey 验证失败';
        fail(message);
        popup.close();
      }
    };

    const closeWatcher = window.setInterval(() => {
      if (!popup.closed) {
        return;
      }
      fail('已取消 Passkey 验证，可重新发起');
    }, 400);

    const timeoutId = window.setTimeout(() => {
      fail('Passkey 验证超时，请重试');
      popup.close();
    }, 90_000);

    window.addEventListener('message', handleMessage);
  });
}

export const passkeyBridgeEvents = {
  ready: PASSKEY_BRIDGE_READY,
  request: PASSKEY_BRIDGE_REQUEST,
  result: PASSKEY_BRIDGE_RESULT,
  error: PASSKEY_BRIDGE_ERROR,
} as const;
