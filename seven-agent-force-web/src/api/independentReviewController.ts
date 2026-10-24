import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';
import { normalizeHumanDecisionResult, normalizeIndependentReviewReport, normalizeReviewRun, normalizeReviewRunPage } from './reviewNormalizers';

type Envelope<T> = { data: T };
export type ReviewDecisionAction = 'ACCEPT_RISK' | 'AUTHORIZE_ONE_REVISION' | 'CANCEL_TASK';
export interface ReviewFinding {
  id?: string;
  severity: string;
  category: string;
  title: string;
  message: string;
  evidence?: { path: string; line: number };
  suggestedDirection?: string;
  reproduction?: string;
  preconditions?: string;
  acceptanceChecks?: string[];
}
export interface ReviewTest { command?: string; exitCode?: number; status: string; summary?: string }
export interface ReviewLimitation { code?: string; summary?: string }
export interface IndependentReviewReport {
  required: boolean; status?: string; verdict?: string; score?: number; summary?: string;
  findings?: ReviewFinding[]; reviewTests?: ReviewTest[]; limitations?: ReviewLimitation[];
  reportDigest?: string; qualityOutcome?: string; automaticRevisionCount: number;
  automaticRevisionLimit: number; noProgressStreak: number; stopReason?: string;
  availableActions: ReviewDecisionAction[]; revisionGuidanceRequired: boolean;
}
export interface HumanReviewDecision { id: API.Int64; action: ReviewDecisionAction; reason: string; revisionGuidance?: string; aiVerdict?: string; reportDigest?: string; qualityOutcome?: string; resultingTaskTurnId?: API.Int64; decidedAt: string }
export interface ReviewRunView {
  id: API.Int64; taskId: API.Int64; taskTurnId: API.Int64; status: string; verdict?: string;
  qualityOutcome?: string; resultTree: string; report?: IndependentReviewReport;
  automaticRevisionNumber: number; noProgressStreak: number; stopReason?: string;
  revisionKind?: string; successorTaskTurnId?: API.Int64; humanDecision?: HumanReviewDecision;
  createdAt: string; finishedAt?: string;
}
export interface ReviewRunPage { items: ReviewRunView[]; nextCursor?: API.Int64 }
export interface HumanDecisionResult { id: API.Int64; taskId: API.Int64; taskTurnId: API.Int64; reviewRunId: API.Int64; action: ReviewDecisionAction; aiVerdict?: string; reportDigest?: string; qualityOutcome?: string; resultingTaskTurnId?: API.Int64; decidedAt: string; revisionGuidance?: string }
const id = (value: API.Int64) => encodeURIComponent(toApiIdParam(value));
export async function getIndependentReview(taskId: API.Int64): Promise<IndependentReviewReport> { return normalizeIndependentReviewReport((await request<Envelope<IndependentReviewReport>>(`/api/workforce/tasks/${id(taskId)}/independent-review`, { method: 'GET' })).data); }
export async function listIndependentReviewRuns(taskId: API.Int64, cursor?: API.Int64): Promise<ReviewRunPage> { const params = new URLSearchParams({ limit: '20' }); if (cursor) params.set('cursor', toApiIdParam(cursor)); return normalizeReviewRunPage((await request<Envelope<ReviewRunPage>>(`/api/workforce/tasks/${id(taskId)}/independent-review/runs?${params}`, { method: 'GET' })).data); }
export async function getIndependentReviewRun(reviewRunId: API.Int64): Promise<ReviewRunView> { return normalizeReviewRun((await request<Envelope<ReviewRunView>>(`/api/workforce/review-runs/${id(reviewRunId)}`, { method: 'GET' })).data); }
export async function decideIndependentReview(reviewRunId: API.Int64, input: { action: ReviewDecisionAction; reason: string; revisionGuidance?: string; idempotencyKey: string }): Promise<HumanDecisionResult> { return normalizeHumanDecisionResult((await request<Envelope<HumanDecisionResult>>(`/api/workforce/review-runs/${id(reviewRunId)}/decisions`, { method: 'POST', data: input })).data); }
