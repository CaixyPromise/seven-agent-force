import { Tabs } from 'antd';
import { useEffect, useState, type ReactNode } from 'react';

type BusinessCard = 'checks' | 'review' | 'result';
export interface ConversationBusinessSlots { checks: ReactNode; review: ReactNode; result: ReactNode }

// These are existing authoritative workspaces, not synthesized conversation items.
// Tabs keep the evidence close without placing three expanded panels in the chat.
export default function ConversationBusinessCards({ checks, review, result, focus }: ConversationBusinessSlots & { focus?: BusinessCard }) {
  const [active, setActive] = useState<BusinessCard>(focus ?? 'checks');
  useEffect(() => {
    if (focus) setActive(focus);
  }, [focus]);
  return <section className="workforce-chat-business" aria-label="任务成果">
    <Tabs
      activeKey={active}
      onChange={(key) => setActive(key as BusinessCard)}
      items={([
        ['checks', '完成检查', checks],
        ['review', '独立审查', review],
        ['result', '结果与下载', result],
      ] as const).filter(([, , content]) => content != null).map(([key, title, content]) => ({
        key,
        label: title,
        children: <div data-business-card={key} className="workforce-chat-business-panel">{content}</div>,
      }))}
    />
  </section>;
}
