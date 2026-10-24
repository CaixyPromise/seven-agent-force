export interface WebAuthnStepHints {
  challenge?: string;
  rpId?: string;
  rpName?: string;
  timeoutSeconds?: number;
  allowCredentialIds?: string[];
  excludeCredentialIds?: string[];
  userHandle?: string;
}

export interface PasskeyRegistrationPayload {
  credentialIdentifier: string;
  clientDataJSON: string;
  attestationObject: string;
  userHandle: string;
  transports?: string[];
  displayName: string;
}

export interface PasskeyAssertionPayload {
  credentialIdentifier: string;
  clientDataJSON: string;
  authenticatorData: string;
  signature: string;
}

const WEBAUTHN_PUBLIC_KEY = 'public-key';

function toUint8Array(value: ArrayBuffer): Uint8Array {
  return new Uint8Array(value);
}

function parseStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => (typeof item === 'string' ? item.trim() : ''))
    .filter((item) => item.length > 0);
}

function readString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function resolveCurrentHostName() {
  if (typeof window === 'undefined' || !window.location?.hostname) {
    return '';
  }
  return window.location.hostname.trim().toLowerCase();
}

function isValidRpIdForHost(rpId: string, hostName: string) {
  if (!rpId || !hostName) {
    return false;
  }
  if (rpId === hostName) {
    return true;
  }
  return hostName.endsWith(`.${rpId}`);
}

function resolveEffectiveRpId(rpIdHint: string) {
  const hostName = resolveCurrentHostName();
  if (!hostName) {
    return rpIdHint;
  }
  if (!rpIdHint) {
    return hostName;
  }
  const normalizedHint = rpIdHint.toLowerCase();
  if (isValidRpIdForHost(normalizedHint, hostName)) {
    return normalizedHint;
  }
  if ((normalizedHint === 'localhost' && hostName === '127.0.0.1')
    || (normalizedHint === '127.0.0.1' && hostName === 'localhost')) {
    return hostName;
  }
  return hostName;
}

function ensureWebAuthnSupport() {
  if (typeof window === 'undefined' || typeof navigator === 'undefined' || !window.PublicKeyCredential) {
    throw new Error('当前浏览器不支持 Passkey/WebAuthn');
  }
}

export function arrayBufferToBase64Url(value: ArrayBuffer): string {
  const bytes = toUint8Array(value);
  let binary = '';
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export function base64UrlToArrayBuffer(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  const paddingLength = (4 - (normalized.length % 4)) % 4;
  const padded = normalized + '='.repeat(paddingLength);
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes.buffer;
}

function toCredentialDescriptors(value: unknown): PublicKeyCredentialDescriptor[] {
  return parseStringArray(value).map((credentialId) => ({
    id: base64UrlToArrayBuffer(credentialId),
    type: WEBAUTHN_PUBLIC_KEY,
  }));
}

export async function createPasskeyRegistrationPayload(
  hints: Record<string, unknown> | undefined,
  userName: string,
  displayName?: string,
): Promise<PasskeyRegistrationPayload> {
  ensureWebAuthnSupport();

  const challenge = readString(hints?.challenge);
  const userHandle = readString(hints?.userHandle);
  const rpId = resolveEffectiveRpId(readString(hints?.rpId));
  const rpName = readString(hints?.rpName) || rpId || 'Seven Framework';
  const timeoutSecondsRaw = Number(hints?.timeoutSeconds);
  const timeout = Number.isFinite(timeoutSecondsRaw) && timeoutSecondsRaw > 0
    ? Math.floor(timeoutSecondsRaw * 1000)
    : 60000;

  if (!challenge || !rpId || !userHandle) {
    throw new Error('Passkey 注册参数不完整，请刷新后重试');
  }

  const finalDisplayName = (displayName || '').trim() || 'My Passkey';

  const createOptions: CredentialCreationOptions = {
    publicKey: {
      challenge: base64UrlToArrayBuffer(challenge),
      rp: {
        id: rpId,
        name: rpName,
      },
      user: {
        id: base64UrlToArrayBuffer(userHandle),
        name: userName || 'user',
        displayName: finalDisplayName,
      },
      pubKeyCredParams: [
        { type: WEBAUTHN_PUBLIC_KEY, alg: -7 },
        { type: WEBAUTHN_PUBLIC_KEY, alg: -257 },
      ],
      timeout,
      authenticatorSelection: {
        residentKey: 'preferred',
        userVerification: 'preferred',
      },
      attestation: 'none',
      excludeCredentials: toCredentialDescriptors(hints?.excludeCredentialIds),
    },
  };

  const credential = (await navigator.credentials.create(createOptions)) as PublicKeyCredential | null;
  if (!credential) {
    throw new Error('Passkey 注册被取消或失败');
  }

  const response = credential.response as AuthenticatorAttestationResponse;
  const anyResponse = response as AuthenticatorAttestationResponse & {
    getTransports?: () => string[];
  };

  return {
    credentialIdentifier: arrayBufferToBase64Url(credential.rawId),
    clientDataJSON: arrayBufferToBase64Url(response.clientDataJSON),
    attestationObject: arrayBufferToBase64Url(response.attestationObject),
    userHandle,
    transports: anyResponse.getTransports?.() ?? [],
    displayName: finalDisplayName,
  };
}

export async function createPasskeyAssertionPayload(
  hints: Record<string, unknown> | undefined,
): Promise<PasskeyAssertionPayload> {
  ensureWebAuthnSupport();

  const challenge = readString(hints?.challenge);
  const rpId = resolveEffectiveRpId(readString(hints?.rpId));
  const timeoutSecondsRaw = Number(hints?.timeoutSeconds);
  const timeout = Number.isFinite(timeoutSecondsRaw) && timeoutSecondsRaw > 0
    ? Math.floor(timeoutSecondsRaw * 1000)
    : 60000;

  if (!challenge || !rpId) {
    throw new Error('Passkey 断言参数不完整，请刷新后重试');
  }

  const allowCredentials = toCredentialDescriptors(hints?.allowCredentialIds);

  const getOptions: CredentialRequestOptions = {
    publicKey: {
      challenge: base64UrlToArrayBuffer(challenge),
      rpId,
      timeout,
      userVerification: 'preferred',
      allowCredentials,
    },
  };

  const credential = (await navigator.credentials.get(getOptions)) as PublicKeyCredential | null;
  if (!credential) {
    throw new Error('Passkey 验证被取消或失败');
  }

  const response = credential.response as AuthenticatorAssertionResponse;
  return {
    credentialIdentifier: arrayBufferToBase64Url(credential.rawId),
    clientDataJSON: arrayBufferToBase64Url(response.clientDataJSON),
    authenticatorData: arrayBufferToBase64Url(response.authenticatorData),
    signature: arrayBufferToBase64Url(response.signature),
  };
}
