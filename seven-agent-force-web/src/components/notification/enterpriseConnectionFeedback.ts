import type {
  EnterpriseConnectionTestResult,
  NotificationChannelType,
  StaticConnectionTestResult,
} from '@/api/notificationController';

type EnterpriseApplicationChannelType = Extract<NotificationChannelType, 'FEISHU_APP' | 'WECOM_APP'>;

export type EnterpriseConnectionGuidance = {
  title: string;
  summary: string;
  steps: string[];
  managementUrl?: string;
};

export type EnterpriseConnectionFeedback = {
  tone: 'success' | 'warning';
  title: string;
  detail?: string;
  guidance?: EnterpriseConnectionGuidance;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object';
}

function hasEnterpriseOutcome(value: unknown): value is EnterpriseConnectionTestResult {
  return isRecord(value) && typeof value.status === 'string';
}

function parseEmbeddedJSON(value: unknown): unknown {
  if (typeof value !== 'string') {
    return value;
  }
  const trimmed = value.trim();
  if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) {
    return value;
  }
  try {
    return JSON.parse(trimmed) as unknown;
  } catch {
    return value;
  }
}

// The API client normally unwraps the common { code, data } envelope. Keep
// this final UI boundary defensive as well: an interceptor retry or a future
// client swap must not turn a provider error into a generic message merely
// because a serialized envelope or one response wrapper reached the mutation
// callback.
function unwrapEnterpriseOutcome(value: unknown): EnterpriseConnectionTestResult {
  let current = parseEmbeddedJSON(value);
  for (let depth = 0; depth < 3; depth += 1) {
    if (hasEnterpriseOutcome(current)) {
      return current;
    }
    if (!isRecord(current) || !isRecord(current.data)) {
      break;
    }
    current = parseEmbeddedJSON(current.data);
  }
  return hasEnterpriseOutcome(current) ? current : { status: 'FAILED' };
}

function providerRejectedMessage(channelType?: EnterpriseApplicationChannelType) {
  return channelType === 'WECOM_APP' ? '企业微信拒绝请求' : '飞书拒绝请求';
}

function failureMessage(result: EnterpriseConnectionTestResult, channelType?: EnterpriseApplicationChannelType) {
  if (result.diagnostic === 'FEISHU_REJECTED') {
    return '飞书拒绝请求';
  }
  if (result.diagnostic === 'WECOM_REJECTED') {
    return '企业微信拒绝请求';
  }

  switch (result.failureClass) {
    case 'AUTHENTICATION':
      return '应用凭据无效';
    case 'INVALID_TARGET':
      return '接收对象不可用或不可见';
    case 'PROVIDER_REJECTED':
      return providerRejectedMessage(channelType);
    case 'CONFIGURATION':
      return '应用配置不完整';
    case 'THROTTLED':
      return '请求过于频繁，请稍后重试';
    case 'TRANSIENT':
      return '连接暂时不可用，请稍后重试';
    default:
      return '测试未通过，请检查应用配置';
  }
}

function sourceMessage(result: EnterpriseConnectionTestResult) {
  const providerError = result.providerError;
  const message = providerError?.message?.trim();
  if (!message) {
    return '';
  }
  const code = providerError?.code?.trim();
  return `${message}${code ? `（${code}）` : ''}`;
}

function wecomTrustedIPGuidance(
  result: EnterpriseConnectionTestResult,
  channelType?: EnterpriseApplicationChannelType,
): EnterpriseConnectionGuidance | undefined {
  if (channelType !== 'WECOM_APP') {
    return undefined;
  }

  const providerError = result.providerError;
  const providerMessage = providerError?.message?.toLowerCase() || '';
  const isTrustedIPRejected =
    providerError?.code?.trim() === '60020' || providerMessage.includes('not allow to access from your ip');
  if (!isTrustedIPRejected) {
    return undefined;
  }

  return {
    title: '请配置企业可信 IP',
    summary: '需要添加的是通知服务访问企业微信时使用的公网出口 IP。',
    steps: [
      '不要填写成员 UserID、应用 AgentId、127.0.0.1 或前端地址。',
      '进入企业微信管理后台：应用管理 → 自建应用 → 当前应用 → 开发者接口 → 企业可信 IP。',
      '保存后回到这里重新发送测试。',
    ],
    managementUrl: 'https://work.weixin.qq.com/wework_admin/frame#apps',
  };
}

export function enterpriseConnectionTestFeedback(
  resultInput: unknown,
  channelType?: EnterpriseApplicationChannelType,
): EnterpriseConnectionFeedback {
  const result = unwrapEnterpriseOutcome(resultInput);
  const warningDetail = result.warnings?.length ? '部分可选项未使用' : '';
  if (result.status === 'PROVIDER_ACCEPTED') {
    return { tone: 'success', title: '平台已受理测试消息', detail: warningDetail || undefined };
  }
  if (result.status === 'UNKNOWN') {
    const detail = sourceMessage(result);
    return {
      tone: 'warning',
      title: '请求结果待确认',
      detail: [detail || '请稍后再试', warningDetail].filter(Boolean).join('；'),
    };
  }
  const detail = sourceMessage(result);
  const guidance = wecomTrustedIPGuidance(result, channelType);
  return {
    tone: 'warning',
    title: guidance?.title || failureMessage(result, channelType),
    detail: [detail, warningDetail].filter(Boolean).join('；') || undefined,
    guidance,
  };
}

export const enterpriseConnectionTestErrorMessage = '测试连接暂时无法完成，请稍后重试';

function hasStaticOutcome(value: unknown): value is StaticConnectionTestResult {
  return isRecord(value) && typeof value.status === 'string';
}

function unwrapStaticOutcome(value: unknown): StaticConnectionTestResult {
  let current = parseEmbeddedJSON(value);
  for (let depth = 0; depth < 3; depth += 1) {
    if (hasStaticOutcome(current)) {
      return current;
    }
    if (!isRecord(current) || !isRecord(current.data)) {
      break;
    }
    current = parseEmbeddedJSON(current.data);
  }
  return hasStaticOutcome(current) ? current : { status: 'FAILED' };
}

function staticSourceMessage(result: StaticConnectionTestResult) {
  const message = result.providerError?.message?.trim();
  const code = result.providerError?.code?.trim();
  if (message) {
    return `${message}${code ? `（${code}）` : ''}`;
  }
  if (typeof result.providerError?.httpStatus === 'number') {
    return `HTTP ${result.providerError.httpStatus}`;
  }
  return '';
}

function staticFailureMessage(result: StaticConnectionTestResult) {
  switch (result.failureClass) {
    case 'DESTINATION_DENIED':
      return '连接地址不可用';
    case 'CONFIGURATION':
      return '连接配置不完整';
    case 'TRANSIENT':
      return '连接暂时不可用，请稍后重试';
    case 'PROVIDER_REJECTED':
      return '接收服务拒绝请求';
    default:
      return '测试未通过，请检查连接配置';
  }
}

// staticConnectionTestFeedback keeps the small management probe truthful: it
// renders only the server's bounded source detail in the form, never a raw
// body, endpoint, credential or global failure toast.
export function staticConnectionTestFeedback(resultInput: unknown): EnterpriseConnectionFeedback {
  const result = unwrapStaticOutcome(resultInput);
  if (result.status === 'PROVIDER_ACCEPTED') {
    return { tone: 'success', title: '连接正常' };
  }
  const detail = staticSourceMessage(result);
  if (result.status === 'UNKNOWN') {
    return { tone: 'warning', title: '请求结果待确认', detail: detail || '请稍后再试' };
  }
  return { tone: 'warning', title: staticFailureMessage(result), detail: detail || undefined };
}
