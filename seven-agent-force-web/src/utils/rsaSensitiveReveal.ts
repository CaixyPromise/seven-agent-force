const KEY_OBFUSCATION_SALT = 0x7a;

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

function base64ToArrayBuffer(base64: string): ArrayBuffer {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

function obfuscatePublicKey(publicKeyBase64: string): string {
  const reversed = publicKeyBase64.split('').reverse().join('');
  let xored = '';
  for (let i = 0; i < reversed.length; i += 1) {
    xored += String.fromCharCode(reversed.charCodeAt(i) ^ KEY_OBFUSCATION_SALT);
  }
  return btoa(xored);
}

export async function buildRevealKeyContext(): Promise<{
  privateKey: CryptoKey;
  obfuscatedClientPublicKey: string;
}> {
  const keyPair = await crypto.subtle.generateKey(
    {
      name: 'RSA-OAEP',
      modulusLength: 2048,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: 'SHA-256',
    },
    true,
    ['encrypt', 'decrypt']
  );

  const publicKeySpki = await crypto.subtle.exportKey('spki', keyPair.publicKey);
  const publicKeyBase64 = arrayBufferToBase64(publicKeySpki);

  return {
    privateKey: keyPair.privateKey,
    obfuscatedClientPublicKey: obfuscatePublicKey(publicKeyBase64),
  };
}

export async function decryptSensitiveValue(encryptedValue: string, privateKey: CryptoKey): Promise<string> {
  const encryptedBuffer = base64ToArrayBuffer(encryptedValue);
  const plainBuffer = await crypto.subtle.decrypt(
    {
      name: 'RSA-OAEP',
    },
    privateKey,
    encryptedBuffer
  );
  return new TextDecoder().decode(plainBuffer);
}
