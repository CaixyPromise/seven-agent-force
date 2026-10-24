import { toApiIdParam } from '@/lib/apiId';

export type TaskDetailFocus = 'evaluation' | 'result' | 'continuation' | 'delivery';

export interface TaskDetailLocation {
  turnId?: API.Int64;
  focus?: TaskDetailFocus;
  deliveryId?: API.Int64;
  reviewId?: API.Int64;
}

const taskDetailFocuses = new Set<TaskDetailFocus>(['evaluation', 'result', 'continuation', 'delivery']);

function parseInt64Query(value: string | null): API.Int64 | undefined {
  if (!value || value !== value.trim()) return undefined;
  try {
    return toApiIdParam(value);
  } catch {
    return undefined;
  }
}

export function parseTaskDetailLocation(searchParams: Pick<URLSearchParams, 'get'>): TaskDetailLocation {
  const turn = searchParams.get('turn');
  const focus = searchParams.get('focus');
  return {
    turnId: parseInt64Query(turn),
    focus: focus && taskDetailFocuses.has(focus as TaskDetailFocus) ? focus as TaskDetailFocus : undefined,
    deliveryId: parseInt64Query(searchParams.get('delivery')),
    reviewId: parseInt64Query(searchParams.get('review')),
  };
}

export function buildTaskDeepLink(
  taskId: API.Int64,
  taskTurnId: API.Int64,
  focus: TaskDetailFocus,
  target?: { deliveryId?: API.Int64; reviewId?: API.Int64 },
): string {
  const normalizedTaskId = toApiIdParam(taskId);
  const normalizedTurnId = toApiIdParam(taskTurnId);
  if (!taskDetailFocuses.has(focus)) {
    throw new TypeError('Unsupported task detail focus');
  }
  const query = new URLSearchParams({ turn: normalizedTurnId, focus });
  if (focus === 'delivery' && target?.deliveryId) query.set('delivery', toApiIdParam(target.deliveryId));
  if (focus === 'delivery' && target?.reviewId) query.set('review', toApiIdParam(target.reviewId));
  return `/workforce/tasks/${encodeURIComponent(normalizedTaskId)}?${query.toString()}`;
}
