const feedback: Record<string, string> = {
  INDEPENDENT_REVIEW_NOT_FOUND: '独立审查记录不存在或不可访问。', INDEPENDENT_REVIEW_ACTION_NOT_ALLOWED: '当前状态不允许执行此操作。',
  INDEPENDENT_REVIEW_STALE: '审查状态已经变化，请刷新后重试。', INDEPENDENT_REVIEW_IDEMPOTENCY_CONFLICT: '相同请求标识已用于不同决定，请确认最新状态。',
  INDEPENDENT_REVIEW_REASON_INVALID: '请填写有效的授权原因（最多 2000 字节）。', INDEPENDENT_REVIEW_GUIDANCE_INVALID: '修订建议格式无效或包含敏感内容。',
  INDEPENDENT_REVIEW_GUIDANCE_REQUIRED: '没有可信审查报告时，必须填写明确的修订建议。', INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE: '当前结果没有可恢复的可信检查点。',
  INDEPENDENT_REVIEW_DETERMINISTIC_AUTHORITY_INVALID: '确定性验证未通过，不能接受当前结果。', INDEPENDENT_REVIEW_STEP_UP_UNAVAILABLE: '二次验证暂不可用，请稍后重试。',
};
export function reviewErrorCode(error: unknown) {
  if (!error || typeof error !== 'object') return '';
  const value = error as {
    reasonCode?: unknown;
    details?: { reasonCode?: unknown };
    payload?: { data?: { reasonCode?: unknown } };
  };
  return String(value.reasonCode ?? value.details?.reasonCode ?? value.payload?.data?.reasonCode ?? '');
}
export function reviewFeedback(error: unknown) { return feedback[reviewErrorCode(error)] ?? '当前操作未完成，请刷新状态后重试。'; }
