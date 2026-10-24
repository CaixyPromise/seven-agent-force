/**
 * @typedef {{ code?: number, message?: string, error?: string, error_description?: string }} LoginErrorPayload
 * @typedef {{ loginTransactionId: string, loginContextId: string }} LoginSession
 */

/**
 * @param {unknown} error
 * @returns {LoginErrorPayload}
 */
function readErrorPayload(error) {
  const payload = error?.payload;
  if (payload && typeof payload === 'object') {
    return payload;
  }
  const responsePayload = error?.response?.data;
  if (responsePayload && typeof responsePayload === 'object') {
    return responsePayload;
  }
  return {};
}

/**
 * @param {unknown} error
 */
export function isExpiredLoginSessionError(error) {
  const payload = readErrorPayload(error);
  const message = `${payload.message || ''} ${payload.error_description || ''}`.trim();
  return payload.error === 'login_required'
    || (payload.code === 40100 && message.includes('登录事务'))
    || (payload.code === 40300 && (
      message.includes('登录上下文')
      || message.includes('登录事务')
    ))
    || (payload.code === 40900 && (
      message.includes('登录上下文')
      || message.includes('登录事务')
    ));
}

/**
 * @param {{
 *   refreshTransaction: () => Promise<string>,
 *   resolveLoginOptions: (loginTransactionId: string) => Promise<{ loginContextId?: string }>,
 * }} dependencies
 * @returns {Promise<LoginSession>}
 */
export async function refreshLoginSession({ refreshTransaction, resolveLoginOptions }) {
  const loginTransactionId = (await refreshTransaction()).trim();
  if (!loginTransactionId) {
    throw new Error('登录事务刷新失败，请稍后重试');
  }
  const loginOptions = await resolveLoginOptions(loginTransactionId);
  const loginContextId = (loginOptions.loginContextId || '').trim();
  if (!loginContextId) {
    throw new Error('登录上下文刷新失败，请稍后重试');
  }
  return { loginTransactionId, loginContextId };
}
