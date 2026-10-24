import { compareDecimal, decimal } from './conversationContract';

/** Parse bounded SSE framing only. Event bodies cannot grant actions or render content. */
export function consumeEventFrames(input: string, cursor: string): { rest: string; after: string; itemIds: string[] } {
  if (input.length > 1024 * 1024) throw new Error('STREAM_FRAME_LIMIT');
  const frames = input.replace(/\r\n/g, '\n').split('\n\n');
  const rest = frames.pop() ?? '';
  let after = cursor;
  const itemIds = new Set<string>();
  for (const frame of frames) {
    if (/^event: ?error$/m.test(frame)) throw new Error('STREAM_UNAVAILABLE');
    const raw = /^id: ?([^\n]*)$/m.exec(frame)?.[1];
    if (!raw) continue;
    const id = decimal(raw);
    if (compareDecimal(id, after) > 0) {
      after = id;
      if (/^event: ?conversation\.updated$/m.test(frame)) {
        const data = frame.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trimStart()).join('\n');
        const event = JSON.parse(data) as { payload?: { itemId?: unknown } };
        itemIds.add(decimal(event.payload?.itemId));
      }
    }
  }
  return { rest, after, itemIds: [...itemIds] };
}
