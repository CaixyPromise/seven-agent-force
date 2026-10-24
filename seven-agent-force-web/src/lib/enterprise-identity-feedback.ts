export const FEISHU_DEVELOPER_CONSOLE_URL = 'https://open.feishu.cn/app';
export const FEISHU_REDIRECT_GUIDE_URL = 'https://open.feishu.cn/community/articles/7317091221654224898?lang=zh-CN';
export const WECOM_ADMIN_APPLICATIONS_URL = 'https://work.weixin.qq.com/wework_admin/frame#apps';
export const WECOM_OAUTH_GUIDE_URL = 'https://developer.work.weixin.qq.com/document/path/91335';
export const WECOM_MESSAGE_GUIDE_URL = 'https://developer.work.weixin.qq.com/document/path/90236';

type ErrorPayload = {
  code?: unknown;
  message?: unknown;
  traceId?: unknown;
  data?: unknown;
};

export type EnterpriseRecoveryAction = 'RETRY' | 'RETURN_TO_SETTINGS' | 'CONTACT_ADMIN' | 'OPEN_ACCOUNT_SECURITY';

export type EnterpriseFailureFeedback = {
  reasonCode?: string;
  title: string;
  description: string;
  action: EnterpriseRecoveryAction;
  traceId?: string;
};

function enterpriseProviderLabel(provider: string | undefined): string {
  switch (provider?.trim().toUpperCase()) {
    case 'FEISHU':
      return '飞书';
    case 'WECOM':
      return '企业微信';
    default:
      return '企业账号';
  }
}

function normalizedPayload(error: unknown): ErrorPayload | undefined {
  const payload = (error as { payload?: unknown } | null | undefined)?.payload;
  return payload && typeof payload === 'object' ? payload as ErrorPayload : undefined;
}

function readReasonCode(payload: ErrorPayload | undefined): string | undefined {
  const data = payload?.data;
  if (!data || typeof data !== 'object') return undefined;
  const value = (data as { reasonCode?: unknown }).reasonCode;
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function readTraceId(payload: ErrorPayload | undefined): string | undefined {
  return typeof payload?.traceId === 'string' && payload.traceId.trim() ? payload.traceId.trim() : undefined;
}

export function enterpriseFailureReason(error: unknown): string | undefined {
  return readReasonCode(normalizedPayload(error));
}

export function enterpriseBindingFeedback(error: unknown, provider?: string): EnterpriseFailureFeedback {
  const payload = normalizedPayload(error);
  const reasonCode = readReasonCode(payload);
  const traceId = readTraceId(payload);
  const providerLabel = enterpriseProviderLabel(provider);
  const result = (title: string, description: string, action: EnterpriseRecoveryAction): EnterpriseFailureFeedback => ({
    reasonCode,
    title,
    description,
    action,
    traceId,
  });

  switch (reasonCode) {
    case 'ENTERPRISE_AUTHORIZATION_STATE_REPLAYED':
    case 'ENTERPRISE_AUTHORIZATION_EXPIRED':
      return result('授权已失效', '这次授权已经使用或超过有效期。请返回账户设置，重新发起一次绑定。', 'RETURN_TO_SETTINGS');
    case 'ENTERPRISE_AUTHORIZATION_OUTCOME_UNKNOWN':
      return result('授权结果暂时无法确认', '平台没有获得可确认的授权结果。请返回账户设置，重新发起一次绑定，不要刷新当前回调页。', 'RETURN_TO_SETTINGS');
    case 'ENTERPRISE_CONNECTION_DRIFT':
      return result('企业连接已更新', `管理员在授权期间更新了${providerLabel}连接。请返回账户设置，使用最新连接重新绑定。`, 'RETURN_TO_SETTINGS');
    case 'ENTERPRISE_AUTHORIZATION_NONCE_MISMATCH':
    case 'ENTERPRISE_AUTHORIZATION_PKCE_MISMATCH':
      return result('授权校验未通过', '本次授权信息与平台发起的请求不一致。请返回账户设置重新绑定；如持续出现，请联系管理员。', 'RETURN_TO_SETTINGS');
    case 'EXTERNAL_IDENTITY_CONFLICT':
      return result('这个企业账号暂时无法绑定', '该企业账号已经绑定到其他平台账号。请确认当前登录账号，或联系管理员处理原有绑定。', 'CONTACT_ADMIN');
    case 'EXTERNAL_IDENTITY_REVOKED':
      return result('企业账号绑定已被撤销', '该企业账号当前不能继续使用。请联系管理员确认账号状态。', 'CONTACT_ADMIN');
    case 'ENTERPRISE_CONNECTION_NOT_CONFIGURED':
    case 'ENTERPRISE_CONNECTION_DISABLED':
    case 'ENTERPRISE_CONNECTION_NOT_VERIFIED':
      return result('组织的企业账号连接暂不可用', `请联系管理员检查${providerLabel}应用连接是否已保存、检查并启用。`, 'CONTACT_ADMIN');
    case 'ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID':
      return result('绑定回调地址尚未就绪', `请联系管理员把平台显示的 OAuth 回调地址添加到${providerLabel}应用后台，然后重新绑定。`, 'CONTACT_ADMIN');
    case 'EXTERNAL_LOGIN_GRANT_REQUIRED':
      return result('尚未获得企业登录权限', '账号绑定和企业登录是两项独立权限。请联系管理员为当前已绑定账号开启企业登录。', 'CONTACT_ADMIN');
    case 'ENTERPRISE_AUTHORIZATION_STATE_INVALID':
      return result(`${providerLabel}授权没有完成`, `授权可能被取消，或${providerLabel}应用配置尚未生效。请返回账户设置重新绑定；如问题持续出现，请联系管理员检查当前应用的凭证、回调地址和可用范围。`, 'RETURN_TO_SETTINGS');
    case 'ENTERPRISE_AUTHORIZATION_PROVIDER_REJECTED':
      return result(`${providerLabel}没有接受本次授权`, providerLabel === '企业微信'
        ? '请确认 Seven 中填写的应用 Agent ID 和应用密钥来自同一个企业微信自建应用，然后重新绑定。'
        : `请确认 Seven 中填写的应用凭证属于当前${providerLabel}应用，然后重新绑定。`, 'RETURN_TO_SETTINGS');
	case 'ENTERPRISE_AUTHORIZATION_WECOM_TRUSTED_IP_REQUIRED':
		return result('需要配置企业可信 IP', '企业微信已经完成扫码回调，但拒绝了 Seven 服务端的公网出口 IP。请联系管理员在当前自建应用的“企业可信 IP”中放行服务端出口 IP，然后重新绑定。', 'CONTACT_ADMIN');
	case 'ENTERPRISE_AUTHORIZATION_WECOM_TRUSTED_DOMAIN_REQUIRED':
		return result('需要配置企业微信可信域名', '请联系管理员检查当前自建应用的“企业微信授权登录”回调域和“网页授权及 JS-SDK”可信域名，确保与平台回调域完全一致。', 'CONTACT_ADMIN');
    case 'ENTERPRISE_AUTHORIZATION_INTERNAL_ERROR':
      return result('平台暂时无法完成绑定', `${providerLabel}已经返回授权结果，但平台处理未完成。请返回账户设置重新绑定；如持续出现，请把参考编号提供给管理员。`, 'RETURN_TO_SETTINGS');
    default:
      return result('平台暂时无法完成绑定', '请返回账户设置重新绑定。若问题持续出现，请把参考编号提供给管理员。', 'RETURN_TO_SETTINGS');
  }
}

export function enterpriseBindingStartFeedback(error: unknown): EnterpriseFailureFeedback {
  const payload = normalizedPayload(error);
  const reasonCode = readReasonCode(payload);
  const traceId = readTraceId(payload);
  const result = (title: string, description: string, action: EnterpriseRecoveryAction): EnterpriseFailureFeedback => ({
    reasonCode,
    title,
    description,
    action,
    traceId,
  });

  switch (reasonCode) {
    case 'ENTERPRISE_CONNECTION_NOT_CONFIGURED':
      return result('组织尚未配置该企业账号', '请联系管理员先在“企业连接”中添加并检查对应应用。', 'CONTACT_ADMIN');
    case 'ENTERPRISE_CONNECTION_DISABLED':
    case 'ENTERPRISE_CONNECTION_NOT_VERIFIED':
      return result('企业账号连接暂不可用', '请联系管理员检查应用凭证、启用成员绑定，并确认连接检查已通过。', 'CONTACT_ADMIN');
    case 'ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID':
      return result('绑定回调地址尚未就绪', '请联系管理员配置平台公开访问地址，并把生成的 OAuth 回调地址添加到飞书开发者后台。', 'CONTACT_ADMIN');
    default:
      return result('暂时无法开始绑定', '请稍后重试；如持续出现，请把参考编号提供给管理员。', 'RETRY');
  }
}
