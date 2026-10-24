import type { WorkforceEvent } from '@/api/workforceController';

export type TaskEventPresentation =
  | { kind: 'event'; event: WorkforceEvent }
  | {
      kind: 'conversation-sync';
      key: string;
      events: WorkforceEvent[];
      summary: string;
      createdAt: string;
      technicalFields: Array<{ sequence: number; kind: string; itemId: unknown; version: unknown }>;
    };

const conversationKindLabels: Record<string, string> = {
  ACTIVITY: 'Agent 活动',
  ASSISTANT_MESSAGE: 'Agent 消息',
  USER_MESSAGE: '用户补充',
  QUESTION: '待回答问题',
  OPERATION: '访问操作',
  DELIVERY: 'Git 交付',
  RESULT: '任务结果',
};

function conversationSyncPresentation(events: WorkforceEvent[]): TaskEventPresentation {
  const counts = new Map<string, number>();
  for (const event of events) {
    const kind = typeof event.payload?.kind === 'string' ? event.payload.kind : 'OTHER';
    counts.set(kind, (counts.get(kind) ?? 0) + 1);
  }
  const summary = [...counts.entries()]
    .map(([kind, count]) => `${conversationKindLabels[kind] ?? '对话内容'} ${count}`)
    .join(' · ');
  return {
    kind: 'conversation-sync',
    key: `conversation-sync-${events[0].sequence}-${events.at(-1)?.sequence ?? events[0].sequence}`,
    events,
    summary,
    createdAt: events[0].createdAt,
    technicalFields: events.map((event) => ({
      sequence: event.sequence,
      kind: typeof event.payload?.kind === 'string' ? event.payload.kind : 'UNKNOWN',
      itemId: event.payload?.itemId,
      version: event.payload?.version,
    })),
  };
}

/**
 * conversation.updated is an authorized invalidation pointer, not a business
 * event. Keep it out of the default timeline and collapse contiguous pointers
 * when an operator explicitly enables technical events.
 */
export function presentTaskEvents(events: WorkforceEvent[], includeTechnical: boolean): TaskEventPresentation[] {
  const ordered = [...events].sort((a, b) => b.sequence - a.sequence);
  if (!includeTechnical) {
    return ordered.filter((event) => event.type !== 'conversation.updated').map((event) => ({ kind: 'event', event }));
  }

  const rows: TaskEventPresentation[] = [];
  for (let index = 0; index < ordered.length;) {
    const event = ordered[index];
    if (event.type !== 'conversation.updated') {
      rows.push({ kind: 'event', event });
      index += 1;
      continue;
    }
    const updates: WorkforceEvent[] = [];
    while (index < ordered.length && ordered[index].type === 'conversation.updated') {
      updates.push(ordered[index]);
      index += 1;
    }
    rows.push(conversationSyncPresentation(updates));
  }
  return rows;
}
