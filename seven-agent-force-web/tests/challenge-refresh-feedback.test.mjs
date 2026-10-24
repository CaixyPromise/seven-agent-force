import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

test('expired email challenge refresh is surfaced without a false resend cooldown', () => {
  const hook = fs.readFileSync(path.join(root, 'src/hooks/useMfaChallengeFlow.ts'), 'utf8');
  const modal = fs.readFileSync(path.join(root, 'src/components/challenge/index.tsx'), 'utf8');
  const otp = fs.readFileSync(
    path.join(root, 'src/components/challenge/components/OtpForm.tsx'),
    'utf8',
  );

  assert.match(hook, /payloadCode === 40400/);
  assert.match(hook, /验证会话已失效，请关闭验证窗口后重新提交操作/);
  assert.match(modal, /setError\(message\)/);
  assert.match(otp, /catch \{[\s\S]*false success cooldown/);
  assert.doesNotMatch(otp, /catch \{[\s\S]*setCountdown\(COOLDOWN_DURATION\)/);
});

test('email OTP separates resend cooldown from verification cooldown', () => {
  const modal = fs.readFileSync(path.join(root, 'src/components/challenge/index.tsx'), 'utf8');

  assert.match(modal, /hints\.resendCooldownSeconds/);
  assert.match(modal, /cooldownSeconds=\{resendCooldownSeconds\}/);
  assert.match(modal, /disabled=\{challengeFlow\.busy \|\| verificationCountdown > 0\}/);
  assert.match(modal, /秒后可重试/);
  assert.match(modal, /form\.setFieldValue\('oneTimePassword', undefined\)/);
});
