import type {
  HumanReviewDecision,
  HumanDecisionResult,
  IndependentReviewReport,
  ReviewFinding,
  ReviewLimitation,
  ReviewRunPage,
  ReviewRunView,
  ReviewTest,
} from './independentReviewController';

const MAX_INT64 = '9223372036854775807';
const reviewStatuses = new Set(['PENDING', 'ASSIGNED', 'RUNNING', 'VALIDATING_REPORT', 'PASSED', 'REVISION_REQUIRED', 'REJECTED', 'RETRY_WAIT', 'MANUAL_REVIEW', 'CANCELLED']);
// Task summaries exist before a ReviewRun; this is not a Run state.
const reviewSummaryStatuses = new Set([...reviewStatuses, 'PENDING_FINALIZATION']);
const verdicts = new Set(['PASS', 'REVISE', 'REJECT']);
const actions = new Set(['ACCEPT_RISK', 'AUTHORIZE_ONE_REVISION', 'CANCEL_TASK']);
const revisionKinds = new Set(['AUTOMATIC', 'HUMAN_AUTHORIZED']);
const findingSeverities = new Set(['BLOCKING', 'WARNING', 'INFO']);
const findingCategories = new Set(['REQUIREMENT', 'CORRECTNESS', 'SECURITY', 'TESTING', 'MAINTAINABILITY']);
const limitationCodes = new Set(['UNABLE_TO_REPRODUCE', 'TIME_BOUNDED', 'NO_PROVIDER_RESPONSE']);
const qualityOutcomes = new Set(['INDEPENDENT_REVIEW_PASSED', 'HUMAN_ACCEPTED']);
const reviewStopReasons = new Set([
  'INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED',
  'INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS',
  'INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE',
  'INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT',
  'INDEPENDENT_REVIEW_REJECTED',
  'INDEPENDENT_REVIEW_REPORT_INVALID',
  'INDEPENDENT_REVIEW_RETRY_EXHAUSTED',
]);
const sha256Digest = /^[0-9a-f]{64}$/;
const gitObjectDigest = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
// The server's review-report/v1 contract uses the past-tense values. Keep the
// legacy short spellings accepted for older task projections, but normalize
// neither into an unbounded string nor into a guessed UI state.
const testStatuses = new Set(['PASSED', 'FAILED', 'SKIPPED', 'UNABLE_TO_REPRODUCE', 'PASS', 'FAIL']);

type RecordValue = Record<string, unknown>;

function record(value: unknown, path: string): RecordValue {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new TypeError(`Invalid review response: ${path}`);
  return value as RecordValue;
}

function stringValue(value: unknown, path: string, options: { optional?: boolean; max?: number } = {}): string | undefined {
  if (value === undefined || value === null) {
    if (options.optional) return undefined;
    throw new TypeError(`Invalid review response: ${path}`);
  }
  if (typeof value !== 'string') throw new TypeError(`Invalid review response: ${path}`);
  if (options.max !== undefined && new TextEncoder().encode(value).length > options.max) throw new TypeError(`Invalid review response: ${path}`);
  return value;
}

function idValue(value: unknown, path: string, optional = false): API.Int64 | undefined {
  if (value === undefined || value === null || value === '' || value === '0') {
    if (optional) return undefined;
    throw new TypeError(`Invalid review response: ${path}`);
  }
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value) || value.length > MAX_INT64.length || (value.length === MAX_INT64.length && value > MAX_INT64)) {
    throw new TypeError(`Invalid review response: ${path}`);
  }
  return value;
}

function numberValue(value: unknown, path: string, optional = false): number | undefined {
  if (value === undefined || value === null) {
    if (optional) return undefined;
    throw new TypeError(`Invalid review response: ${path}`);
  }
  if (typeof value !== 'number' || !Number.isSafeInteger(value)) throw new TypeError(`Invalid review response: ${path}`);
  return value;
}

function booleanValue(value: unknown, path: string): boolean {
  if (typeof value !== 'boolean') throw new TypeError(`Invalid review response: ${path}`);
  return value;
}

function enumValue<T extends string>(value: unknown, allowed: Set<string>, path: string, optional = false): T | undefined {
  if (value === undefined || value === null || value === '') {
    if (optional) return undefined;
    throw new TypeError(`Invalid review response: ${path}`);
  }
  if (typeof value !== 'string' || !allowed.has(value)) throw new TypeError(`Invalid review response: ${path}`);
  return value as T;
}

function arrayValue(value: unknown, path: string): unknown[] {
  if (!Array.isArray(value)) throw new TypeError(`Invalid review response: ${path}`);
  return value;
}

function boundedArray(value: unknown, path: string, max: number): unknown[] {
  const items = arrayValue(value, path);
  if (items.length > max) throw new TypeError(`Invalid review response: ${path}`);
  return items;
}

function optionalText(value: unknown, path: string, max = 20000) {
  return stringValue(value, path, { optional: true, max });
}

function digestValue(value: unknown, path: string, optional = false): string | undefined {
  if (value === undefined || value === null || value === '') {
    if (optional) return undefined;
    throw new TypeError(`Invalid review response: ${path}`);
  }
  const digest = stringValue(value, path, { max: 64 })!;
  if (!sha256Digest.test(digest)) throw new TypeError(`Invalid review response: ${path}`);
  return digest;
}

function nonNegativeNumber(value: unknown, path: string, optional = false): number | undefined {
  const number = numberValue(value, path, optional);
  if (number !== undefined && number < 0) throw new TypeError(`Invalid review response: ${path}`);
  return number;
}

function evidence(value: unknown, path: string): ReviewFinding['evidence'] | undefined {
  if (value === undefined || value === null) return undefined;
  const item = record(value, path);
  const file = stringValue(item.path, `${path}.path`, { max: 1024 })!;
  if (!file || file.startsWith('/') || file.includes('\\') || file.split('/').some((part) => part === '..' || part === '')) throw new TypeError(`Invalid review response: ${path}.path`);
  const line = numberValue(item.line, `${path}.line`)!;
  if (line < 1) throw new TypeError(`Invalid review response: ${path}.line`);
  return { path: file, line };
}

function normalizeFinding(value: unknown, index: number): ReviewFinding {
  const item = record(value, `findings[${index}]`);
  const acceptanceChecks = item.acceptanceChecks === undefined || item.acceptanceChecks === null
    ? undefined
    : boundedArray(item.acceptanceChecks, `findings[${index}].acceptanceChecks`, 16)
      .map((check, checkIndex) => stringValue(check, `findings[${index}].acceptanceChecks[${checkIndex}]`, { max: 1000 })!);
  return {
    id: optionalText(item.id, `findings[${index}].id`, 128),
    severity: enumValue(item.severity, findingSeverities, `findings[${index}].severity`)!,
    category: enumValue(item.category, findingCategories, `findings[${index}].category`)!,
    title: stringValue(item.title, `findings[${index}].title`, { max: 400 })!,
    message: stringValue(item.message, `findings[${index}].message`, { max: 10000 })!,
    evidence: evidence(item.evidence, `findings[${index}].evidence`),
    suggestedDirection: optionalText(item.suggestedDirection, `findings[${index}].suggestedDirection`, 4000),
    reproduction: optionalText(item.reproduction, `findings[${index}].reproduction`, 4000),
    preconditions: optionalText(item.preconditions, `findings[${index}].preconditions`, 4000),
    acceptanceChecks,
  };
}

function normalizeTest(value: unknown, index: number): ReviewTest {
  const item = record(value, `reviewTests[${index}]`);
  return {
    command: optionalText(item.command, `reviewTests[${index}].command`, 300),
    exitCode: numberValue(item.exitCode, `reviewTests[${index}].exitCode`, true),
    status: enumValue(item.status, testStatuses, `reviewTests[${index}].status`)!,
    summary: optionalText(item.summary, `reviewTests[${index}].summary`, 2000),
  };
}

function normalizeLimitation(value: unknown, index: number): ReviewLimitation {
  const item = record(value, `limitations[${index}]`);
  return { code: enumValue(item.code, limitationCodes, `limitations[${index}].code`, true), summary: optionalText(item.summary, `limitations[${index}].summary`, 3000) };
}

export function normalizeIndependentReviewReport(value: unknown): IndependentReviewReport {
  const item = record(value, 'independentReview');
  const findings = boundedArray(item.findings ?? [], 'findings', 100).map(normalizeFinding);
  const reviewTests = boundedArray(item.reviewTests ?? [], 'reviewTests', 100).map(normalizeTest);
  const limitations = boundedArray(item.limitations ?? [], 'limitations', 100).map(normalizeLimitation);
  const availableActions = boundedArray(item.availableActions, 'availableActions', 3).map((action, index) => enumValue<IndependentReviewReport['availableActions'][number]>(action, actions, `availableActions[${index}]`)!);
  const verdict = enumValue<string>(item.verdict, verdicts, 'verdict', true);
  return {
    required: booleanValue(item.required, 'required'),
    status: enumValue<string>(item.status, reviewSummaryStatuses, 'status', true),
    verdict,
    score: (() => {
      const score = numberValue(item.score, 'score', true);
      if (score !== undefined && (score < 0 || score > 100)) throw new TypeError('Invalid review response: score');
      return score;
    })(),
    summary: optionalText(item.summary, 'summary', 10000),
    findings,
    reviewTests,
    limitations,
    reportDigest: digestValue(item.reportDigest, 'reportDigest', true),
    qualityOutcome: enumValue(item.qualityOutcome, qualityOutcomes, 'qualityOutcome', true),
    automaticRevisionCount: nonNegativeNumber(item.automaticRevisionCount, 'automaticRevisionCount')!,
    automaticRevisionLimit: nonNegativeNumber(item.automaticRevisionLimit, 'automaticRevisionLimit')!,
    noProgressStreak: nonNegativeNumber(item.noProgressStreak, 'noProgressStreak')!,
    stopReason: enumValue(item.stopReason, reviewStopReasons, 'stopReason', true),
    availableActions,
    revisionGuidanceRequired: booleanValue(item.revisionGuidanceRequired, 'revisionGuidanceRequired'),
  };
}

function normalizeDecision(value: unknown): HumanReviewDecision {
  const item = record(value, 'humanDecision');
  return {
    id: idValue(item.id, 'humanDecision.id')!,
    action: enumValue<HumanReviewDecision['action']>(item.action, actions, 'humanDecision.action')!,
    reason: stringValue(item.reason, 'humanDecision.reason', { max: 2000 })!,
    revisionGuidance: optionalText(item.revisionGuidance, 'humanDecision.revisionGuidance', 4000),
    aiVerdict: enumValue<string>(item.aiVerdict, verdicts, 'humanDecision.aiVerdict', true),
    reportDigest: digestValue(item.reportDigest, 'humanDecision.reportDigest', true),
    qualityOutcome: enumValue(item.qualityOutcome, qualityOutcomes, 'humanDecision.qualityOutcome', true),
    resultingTaskTurnId: idValue(item.resultingTaskTurnId, 'humanDecision.resultingTaskTurnId', true),
    decidedAt: stringValue(item.decidedAt, 'humanDecision.decidedAt', { max: 64 })!,
  };
}

export function normalizeReviewRun(value: unknown): ReviewRunView {
  const item = record(value, 'reviewRun');
  const resultTree = stringValue(item.resultTree, 'reviewRun.resultTree', { optional: true, max: 64 }) ?? '';
  if (resultTree && !gitObjectDigest.test(resultTree)) throw new TypeError('Invalid review response: reviewRun.resultTree');
  return {
    id: idValue(item.id, 'reviewRun.id')!,
    taskId: idValue(item.taskId, 'reviewRun.taskId')!,
    taskTurnId: idValue(item.taskTurnId, 'reviewRun.taskTurnId')!,
    status: enumValue<string>(item.status, reviewStatuses, 'reviewRun.status')!,
    verdict: enumValue<string>(item.verdict, verdicts, 'reviewRun.verdict', true),
    qualityOutcome: enumValue(item.qualityOutcome, qualityOutcomes, 'reviewRun.qualityOutcome', true),
    // A newly assigned/running ReviewRun has no ResultTree yet.  Preserve the
    // empty projection for that non-terminal state and still validate every
    // non-empty tree digest strictly.
    resultTree,
    report: item.report === undefined || item.report === null ? undefined : normalizeIndependentReviewReport(item.report),
    automaticRevisionNumber: nonNegativeNumber(item.automaticRevisionNumber, 'reviewRun.automaticRevisionNumber')!,
    noProgressStreak: nonNegativeNumber(item.noProgressStreak, 'reviewRun.noProgressStreak')!,
    stopReason: enumValue(item.stopReason, reviewStopReasons, 'reviewRun.stopReason', true),
    revisionKind: enumValue<string>(item.revisionKind, revisionKinds, 'reviewRun.revisionKind', true),
    successorTaskTurnId: idValue(item.successorTaskTurnId, 'reviewRun.successorTaskTurnId', true),
    humanDecision: item.humanDecision === undefined || item.humanDecision === null ? undefined : normalizeDecision(item.humanDecision),
    createdAt: stringValue(item.createdAt, 'reviewRun.createdAt', { max: 64 })!,
    finishedAt: optionalText(item.finishedAt, 'reviewRun.finishedAt', 64),
  };
}

export function normalizeReviewRunPage(value: unknown): ReviewRunPage {
  const item = record(value, 'reviewRunPage');
  const items = boundedArray(item.items, 'reviewRunPage.items', 50).map(normalizeReviewRun);
  return { items, nextCursor: idValue(item.nextCursor, 'reviewRunPage.nextCursor', true) };
}

export function normalizeHumanDecisionResult(value: unknown): HumanDecisionResult {
  const item = record(value, 'humanDecisionResult');
  const reportDigest = digestValue(item.reportDigest, 'humanDecisionResult.reportDigest', true);
  return {
    id: idValue(item.id, 'humanDecisionResult.id')!,
    taskId: idValue(item.taskId, 'humanDecisionResult.taskId')!,
    taskTurnId: idValue(item.taskTurnId, 'humanDecisionResult.taskTurnId')!,
    reviewRunId: idValue(item.reviewRunId, 'humanDecisionResult.reviewRunId')!,
    action: enumValue<HumanDecisionResult['action']>(item.action, actions, 'humanDecisionResult.action')!,
    aiVerdict: enumValue<string>(item.aiVerdict, verdicts, 'humanDecisionResult.aiVerdict', true),
    reportDigest,
    qualityOutcome: enumValue<string>(item.qualityOutcome, qualityOutcomes, 'humanDecisionResult.qualityOutcome', true),
    resultingTaskTurnId: idValue(item.resultingTaskTurnId, 'humanDecisionResult.resultingTaskTurnId', true),
    decidedAt: stringValue(item.decidedAt, 'humanDecisionResult.decidedAt', { max: 64 })!,
    revisionGuidance: optionalText(item.revisionGuidance, 'humanDecisionResult.revisionGuidance', 4000),
  };
}
