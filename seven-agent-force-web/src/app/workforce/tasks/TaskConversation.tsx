import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Alert, Button, Checkbox, Collapse, Empty, Form, Input, Radio, Skeleton, Space, Typography } from 'antd';
import VirtualList, { type ListRef } from '@rc-component/virtual-list';
import { useSearchParams } from 'react-router-dom';
import { useAuthStore } from '@/store/auth';
import { getConversation, getConversationItem, markConversationRead, respondToInteraction, sendConversationMessage, watchConversation } from '@/api/conversationController';
import { compareDecimal, conversationFeedback, conversationText, decimal, mergeConversation, validateAnswers, type Answer, type ConversationItem, type ConversationPage, type QuestionBody } from '@/api/conversationContract';
import { ConversationDrafts } from './conversationDrafts';
import NetworkOperationCard from './NetworkOperationCard';
import GitDeliveryCard from './GitDeliveryCard';

const drafts = new ConversationDrafts();
const authScope = () => { const u = useAuthStore.getState().user; return `${u?.id ?? ''}:${u?.primaryOrgId ?? ''}:${u?.authVersion ?? ''}`; };
drafts.setScope(authScope());
useAuthStore.subscribe(() => drafts.setScope(authScope()));

type ConversationProps = { taskId: string; goal: string; onResults: () => void; onDetails: () => void; onDeliveryChange?: () => void; milestones?: ReactNode; reviewPending?: boolean; onReview?: () => void; visible?: boolean };
export default function TaskConversation({ taskId, goal, onResults, onDetails, onDeliveryChange, milestones, reviewPending, onReview, visible = true }: ConversationProps) {
  const scope = useAuthStore((s) => `${s.user?.id ?? ''}:${s.user?.primaryOrgId ?? ''}:${s.user?.authVersion ?? ''}`);
  return <ConversationWorkspace key={`${scope}:${taskId}`} taskId={taskId} goal={goal} onResults={onResults} onDetails={onDetails} onDeliveryChange={onDeliveryChange} milestones={milestones} reviewPending={reviewPending} onReview={onReview} visible={visible} />;
}

function ConversationWorkspace({ taskId, goal, onResults, onDetails, onDeliveryChange, milestones, reviewPending, onReview, visible = true }: ConversationProps) {
  const [params] = useSearchParams();
  const [page, setPage] = useState<ConversationPage>();
  const [items, setItems] = useState<ConversationItem[]>([]);
  const displayItems = useMemo(() => groupActivities(items), [items]);
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [older, setOlder] = useState(false);
  const olderBusy = useRef(false);
  const [updates, setUpdates] = useState(false);
  const [active, setActive] = useState<ConversationItem>();
  const [dismissedInteraction, setDismissedInteraction] = useState('');
  const [interactionBusy, setInteractionBusy] = useState(false);
  const [historyHeight, setHistoryHeight] = useState(360);
  const history = useRef<HTMLDivElement>(null);
  const [messageDraft, setMessageDraft] = useState(() => drafts.get(`${taskId}:message`).text);
  const [sending, setSending] = useState(false);
  const sendingRef = useRef(false);
  const [sendError, setSendError] = useState('');
  const list = useRef<ListRef>(null);
  const editor = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const anchor = useRef<{ key: string; offset: number } | undefined>(undefined);
  const follow = useRef(true);
  const abort = useRef<AbortController | undefined>(undefined);
  const refreshPending = useRef<Promise<void> | undefined>(undefined);
  const watermark = useRef('0');
  const refresh = useCallback((itemIds: string[] = []): Promise<void> => {
    if (refreshPending.current) return itemIds.length ? refreshPending.current.then(() => refresh(itemIds)) : refreshPending.current;
    const signal = abort.current?.signal;
    const operation = async () => {
      try {
        const latest = await getConversation(taskId, undefined, signal);
        const reconcileIds = itemIds.length
          ? itemIds
          : itemsRef.current.filter((i) => i.type === 'QUESTION' || i.type === 'OPERATION' || i.type === 'DELIVERY').map((i) => i.id);
        const ids = [...new Set(reconcileIds)].filter((id) => !latest.items.some((n) => n.id === id));
        const refreshed: ConversationItem[] = [];
        // Bound concurrency; old cards can change even when their IDs precede the latest page.
        for (let start = 0; start < ids.length; start += 4) refreshed.push(...await Promise.all(ids.slice(start, start + 4).map((id) => getConversationItem(taskId, id, signal))));
        if (signal?.aborted) return;
        const incoming = [...latest.items, ...refreshed];
        const currentById = new Map(itemsRef.current.map((item) => [item.id, item]));
        const changedItems = incoming.filter((item) => { const old = currentById.get(item.id); return !old || compareDecimal(item.version, old.version) > 0; });
        const changed = changedItems.length > 0;
        follow.current = atBottom.current;
        if (changed) {
          setItems((current) => mergeConversation(current, incoming));
          if (changedItems.some((item) => item.type === 'DELIVERY')) onDeliveryChange?.();
        }
        watermark.current = latest.watermark;
        setPage((current) => current ? { ...latest, nextCursor: current.nextCursor, watermark: compareDecimal(latest.watermark, current.watermark) >= 0 ? latest.watermark : current.watermark } : latest);
        if (changed && !atBottom.current) setUpdates(true);
        setError('');
      } catch (e) { if (!signal?.aborted) { setError(conversationFeedback(e)); throw e; } }
      finally { if (!signal?.aborted) setLoading(false); }
    };
    const promise = operation().finally(() => { if (refreshPending.current === promise) refreshPending.current = undefined; });
    refreshPending.current = promise;
    return promise;
  }, [onDeliveryChange, taskId]);
  useEffect(() => {
    const controller = new AbortController();
    abort.current = controller;
    let stop: (() => void) | undefined;
    void refresh().catch(() => undefined).then(() => {
      if (!controller.signal.aborted) stop = watchConversation(taskId, refresh, () => setError('更新暂时中断，正在重新连接。'), watermark.current);
    });
    return () => { controller.abort(); refreshPending.current = undefined; stop?.(); };
  }, [refresh, taskId]);

  const focusItem = params.get('item');
  useEffect(() => {
    if (!focusItem) return;
    const controller = new AbortController();
    try { decimal(focusItem); } catch { return; }
    void getConversationItem(taskId, focusItem, controller.signal).then((item) => {
      if (controller.signal.aborted) return;
      follow.current = false;
      atBottom.current = false;
      setItems((current) => mergeConversation(current, [item]));
      anchor.current = { key: item.id, offset: 0 };
    }).catch((e) => { if (!controller.signal.aborted) setError(conversationFeedback(e)); });
    return () => controller.abort();
  }, [taskId, focusItem]);

  useLayoutEffect(() => {
    if (anchor.current) { list.current?.scrollTo({ ...anchor.current, align: 'top' }); anchor.current = undefined; }
    else if ((follow.current || atBottom.current) && displayItems.length) list.current?.scrollTo({ key: displayItems[displayItems.length - 1].id, align: 'bottom' });
    follow.current = false;
  }, [displayItems, historyHeight]);
  useEffect(() => { if (active) editor.current?.querySelector<HTMLElement>('textarea,input,button')?.focus(); }, [active]);

  const loadOlder = async () => {
    if (!page?.nextCursor || olderBusy.current) return;
    olderBusy.current = true; setOlder(true);
    const cursor = page.nextCursor;
    const holder = list.current?.nativeElement.querySelector('.rc-virtual-list-holder');
    const top = holder?.getBoundingClientRect().top ?? 0;
    const first = Array.from(list.current?.nativeElement.querySelectorAll<HTMLElement>('[data-conversation-id]') ?? []).find((el) => el.getBoundingClientRect().bottom > top);
    try {
      const next = await getConversation(taskId, cursor, abort.current?.signal);
      if (abort.current?.signal.aborted) return;
      if (next.nextCursor && compareDecimal(next.nextCursor, cursor) >= 0) throw new Error('CONVERSATION_INVALID');
      if (first) anchor.current = { key: first.dataset.conversationId!, offset: first.getBoundingClientRect().top - top };
      follow.current = false;
      setItems((current) => mergeConversation(current, next.items));
      setPage((current) => current ? { ...current, nextCursor: next.nextCursor } : current);
      setError('');
    } catch (e) { if (!abort.current?.signal.aborted) setError(conversationFeedback(e, '历史加载')); }
    finally { olderBusy.current = false; setOlder(false); }
  };
  const latestActive = items.find((i) => i.id === active?.id);
  const interactionKey = (item: ConversationItem) => `${item.id}:${item.version}`;
  const closeInteraction = () => { if (interactionBusy) return; setDismissedInteraction(active ? interactionKey(active) : 'review'); setActive(undefined); };
  const pendingInteraction = items.find((item) =>
    (item.type === 'QUESTION' && item.body.status === 'OPEN' && page?.allowedActions.includes('ANSWER')) ||
    (item.type === 'OPERATION' && item.body.status === 'WAITING_APPROVAL' && page?.allowedActions.includes('DECIDE_OPERATION')) ||
    (item.type === 'DELIVERY' && item.body.status === 'WAITING_USER_DECISION' && page?.allowedActions.includes('DECIDE_DELIVERY') && item.body.allowedActions.length > 0));
  useEffect(() => {
    if (!active && pendingInteraction && interactionKey(pendingInteraction) !== dismissedInteraction) setActive(pendingInteraction);
  }, [active, pendingInteraction, dismissedInteraction]);
  useLayoutEffect(() => {
    const element = history.current;
    if (!element) return;
    const resize = () => setHistoryHeight(Math.max(120, element.clientHeight));
    resize();
    const observer = new ResizeObserver(resize); observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const composing = useRef(false);
  const canAnswer = latestActive?.type === 'QUESTION' && latestActive.body.status === 'OPEN' && active?.type === 'QUESTION' && latestActive.body.version === active.body.version && page?.allowedActions.includes('ANSWER') === true && !error;
  const send = async () => {
    if (sendingRef.current || !messageDraft.trim() || !page?.allowedActions.includes('SEND_MESSAGE') || error) return;
    const text = messageDraft;
    try { conversationText(text); } catch { setSendError('消息内容过长或包含无效字符，请缩短后重试。'); return; }
    const key = `${taskId}:message`;
    const idempotencyKey = drafts.submission(key, text, () => crypto.randomUUID());
    sendingRef.current = true; setSending(true); setSendError('');
    try {
      await sendConversationMessage(taskId, text, idempotencyKey);
      if (abort.current?.signal.aborted) return;
      drafts.clear(key); setMessageDraft('');
      await refresh();
    } catch (e) { if (!abort.current?.signal.aborted) setSendError(conversationFeedback(e, '消息发送')); }
    finally { sendingRef.current = false; setSending(false); }
  };
  const readWatermark = page?.watermark;
  useEffect(() => {
    if (!visible || !readWatermark || document.visibilityState !== 'visible') return;
    void markConversationRead(taskId, readWatermark).catch(() => undefined);
  }, [readWatermark, taskId, visible]);

  const conversation = <div className="workforce-chat-conversation">
    {error && <Alert className="workforce-chat-local-alert" type="warning" title={error} role="alert" action={<Button size="small" loading={loading} onClick={() => { setLoading(true); void refresh().catch(() => undefined); }}>重试</Button>} />}
    <details className="workforce-chat-goal-card">
      <summary>查看完整目标</summary>
      <Typography.Paragraph>{goal}</Typography.Paragraph>
    </details>
    <div ref={history} className="workforce-chat-history">
    {page?.nextCursor && <Button className="workforce-chat-older" type="text" loading={older} onClick={() => void loadOlder()}>加载更早内容</Button>}
    {loading && !page ? <Skeleton active /> : !displayItems.length ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="等待 Agent 更新" /> : <VirtualList ref={list} data={displayItems} height={historyHeight} itemHeight={88} itemKey="id" onScroll={(event) => {
      const el = event.currentTarget;
      atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
      if (atBottom.current) setUpdates(false);
    }}>
      {(item) => <article data-conversation-id={item.id} className={`workforce-chat-item workforce-chat-${item.type.toLowerCase()}`} tabIndex={0} aria-label={item.type === 'QUESTION' ? 'Agent 的问题' : item.type === 'OPERATION' ? '外部服务访问请求' : item.type === 'DELIVERY' ? 'Git 交付' : '消息'}>
        <div className="workforce-chat-item-meta"><span>{conversationActor(item)}</span><time dateTime={item.createdAt}>{formatConversationTime(item.createdAt)}</time></div>
        {item.type === 'QUESTION' ? <>
          <div className="workforce-chat-question-heading"><Typography.Text strong>{item.body.isBlocking ? '需要你回答' : 'Agent 想听听你的意见'}</Typography.Text><span className={`workforce-chat-question-state is-${item.body.status.toLowerCase()}`}>{({ OPEN: item.body.isBlocking ? '等待回答后继续' : '可稍后回答', RESOLVED: '已回答', EXPIRED: '已过期', CANCELLED: '已取消', SUPERSEDED: '已关闭' } as Record<string, string>)[item.body.status]}</span></div>
          <Typography.Title level={5}>{item.body.questions.map((q) => q.title).join(' · ')}</Typography.Title>
          {item.body.questions.length === 1 && item.body.questions[0]?.description && <Typography.Paragraph type="secondary">{item.body.questions[0].description}</Typography.Paragraph>}
          {item.body.response?.answers.map((a) => <Typography.Paragraph key={a.questionKey} className="workforce-chat-answer">{a.selectedOptionKeys.map((k) => item.body.questions.find((q) => q.key === a.questionKey)?.options.find((o) => o.key === k)?.label ?? '').filter(Boolean).join('、')} {a.text}</Typography.Paragraph>)}
          <DeliveryStatus value={item.body.deliveryStatus} closed={['EXPIRED', 'CANCELLED', 'SUPERSEDED'].includes(item.body.status)} />
          {item.body.status === 'OPEN' && page?.allowedActions.includes('ANSWER') && active?.id !== item.id && <Button type="link" disabled={!!error || interactionBusy} onClick={() => setActive(item)}>回答问题</Button>}
        </> : item.type === 'OPERATION' && item.body.status === 'WAITING_APPROVAL' ? <><Typography.Paragraph>需要访问外部服务：{item.body.purpose} · 请在下方决定</Typography.Paragraph>{active?.id !== item.id && <Button type="link" disabled={!!error || interactionBusy || !page?.allowedActions.includes('DECIDE_OPERATION')} onClick={() => setActive(item)}>处理访问请求</Button>}</> : item.type === 'OPERATION' ? <NetworkOperationCard taskId={taskId} item={item.body} disabled={!!error || page?.allowedActions.includes('DECIDE_OPERATION') !== true} onChanged={refresh} /> : item.type === 'DELIVERY' && item.body.status === 'WAITING_USER_DECISION' ? <><Typography.Paragraph>Git 交付：{item.body.summary} · 请在下方决定</Typography.Paragraph>{active?.id !== item.id && <Button type="link" disabled={!!error || interactionBusy || !page?.allowedActions.includes('DECIDE_DELIVERY')} onClick={() => setActive(item)}>处理交付决定</Button>}</> : item.type === 'DELIVERY' ? <GitDeliveryCard taskId={taskId} item={item.body} disabled={!!error} pageActions={page?.allowedActions ?? []} onChanged={refresh} onDetails={onDetails} /> : item.type === 'UNSUPPORTED' ? <Typography.Text type="secondary">内容类型暂不支持</Typography.Text> : item.type === 'ACTIVITY' ? <Collapse ghost items={[{ key: item.id, label: item.activityCount > 1 ? `查看 ${item.activityCount} 项执行记录` : activityTitle(item.body.text), children: <Typography.Paragraph className="workforce-chat-text">{item.body.text}</Typography.Paragraph> }]} /> : <>
          {item.type === 'USER_MESSAGE' && item.body.status === 'CANCELLED' && <Typography.Text type="secondary">已取消{item.body.deliveryStatus === 'SAVED' ? ' · 未送达' : ''}</Typography.Text>}
          <Typography.Paragraph className="workforce-chat-text">{item.body.text}</Typography.Paragraph>
          <DeliveryStatus value={item.body.deliveryStatus} closed={item.type === 'USER_MESSAGE' && item.body.status === 'CANCELLED'} />
          {item.type === 'RESULT' && <Button type="link" onClick={onResults}>查看结果</Button>}
        </>}
      </article>}
    </VirtualList>}
    </div>
    {milestones}
    {updates && <Button className="workforce-chat-updates" onClick={() => { list.current?.scrollTo({ index: displayItems.length - 1, align: 'bottom' }); atBottom.current = true; setUpdates(false); }}>查看新进展</Button>}
    <div ref={editor} className={`workforce-chat-composer${active ? ' is-answering' : ''}`}>
      {active?.type === 'QUESTION' ? <QuestionEditor key={`${active.body.interactionId}:${active.body.version}`} taskId={taskId} body={active.body} canAnswer={!!canAnswer} onClose={closeInteraction} onBusyChange={setInteractionBusy} onRefresh={refresh} onSaved={async () => { await refresh(); setActive(undefined); }} /> : active?.type === 'OPERATION' ? <>
        <NetworkOperationCard taskId={taskId} item={latestActive?.type === 'OPERATION' ? latestActive.body : active.body} disabled={!!error || latestActive?.type !== 'OPERATION' || page?.allowedActions.includes('DECIDE_OPERATION') !== true} onBusyChange={setInteractionBusy} onChanged={async () => { await refresh(); setActive(undefined); }} />
        <Button type="link" disabled={interactionBusy} onClick={closeInteraction}>返回对话</Button>
      </> : active?.type === 'DELIVERY' ? <>
        <GitDeliveryCard taskId={taskId} item={latestActive?.type === 'DELIVERY' ? latestActive.body : active.body} disabled={!!error || latestActive?.type !== 'DELIVERY'} pageActions={page?.allowedActions ?? []} onBusyChange={setInteractionBusy} onChanged={async () => { await refresh(); setActive(undefined); }} onDetails={onDetails} />
        <Button type="link" disabled={interactionBusy} onClick={closeInteraction}>返回对话</Button>
      </> : reviewPending && dismissedInteraction !== 'review' ? <>
        <Typography.Paragraph>独立审查需要你的决定。请查看报告后，使用审查工作区中的可用操作。</Typography.Paragraph>
        <Button type="primary" onClick={onReview ?? onResults}>查看独立审查</Button><Button type="link" disabled={interactionBusy} onClick={closeInteraction}>返回对话</Button>
      </> : <>
        <label className="sr-only" htmlFor="task-chat-message">补充任务要求</label>
        <Input.TextArea id="task-chat-message" value={messageDraft} maxLength={20000} autoSize={{ minRows: 1, maxRows: 8 }} disabled={sending || !page?.allowedActions.includes('SEND_MESSAGE') || !!error} placeholder="补充任务要求…" onCompositionStart={() => { composing.current = true; }} onCompositionEnd={() => { composing.current = false; }} onKeyDown={(event) => {
          if (event.key !== 'Enter' || event.shiftKey || composing.current || event.nativeEvent.isComposing) return;
          event.preventDefault(); void send();
        }} onChange={(e) => { setMessageDraft(e.target.value); drafts.set(`${taskId}:message`, { ...drafts.get(`${taskId}:message`), text: e.target.value }); }} />
        {sendError && <div role="alert" className="workforce-chat-field-error">{sendError}</div>}
        <div className="workforce-chat-composer-actions"><Typography.Text type="secondary">Enter 发送 · Shift + Enter 换行</Typography.Text><Button type="primary" loading={sending} disabled={!page?.allowedActions.includes('SEND_MESSAGE') || !!error || !messageDraft.trim()} onClick={() => void send()}>发送</Button></div>
      </>}
    </div>
  </div>;
  return <section className="workforce-chat" aria-label="任务工作区" data-visual-surface="task-workspace">
    {conversation}
  </section>;
}

function conversationActor(item: ConversationItem): string {
  if (item.type === 'USER_MESSAGE') return '你';
  if (item.type === 'RESULT') return '结果';
  if (item.type === 'ACTIVITY') return '执行修复与验证';
  if (item.type === 'QUESTION') return 'Agent 提问';
  if (item.type === 'OPERATION') return '访问请求';
  if (item.type === 'DELIVERY') return 'Git 交付';
  return 'Agent';
}

function formatConversationTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value));
}

function activityTitle(value: string): string {
  const first = value.split('\n').map((part) => part.trim()).find(Boolean);
  return first && first.length <= 72 ? first : '执行活动';
}

type ConversationDisplayItem = ConversationItem & { activityCount: number };

function groupActivities(items: ConversationItem[]): ConversationDisplayItem[] {
  const grouped: ConversationDisplayItem[] = [];
  for (const item of items) {
    const previous = grouped.at(-1);
    if (item.type === 'ACTIVITY' && previous?.type === 'ACTIVITY') {
      grouped[grouped.length - 1] = {
        ...previous,
        id: item.id,
        version: item.version,
        body: { ...previous.body, text: `${previous.body.text}\n\n${item.body.text}` },
        activityCount: previous.activityCount + 1,
      };
      continue;
    }
    grouped.push({ ...item, activityCount: 1 });
  }
  return grouped;
}

function QuestionEditor({ taskId, body, canAnswer, onClose, onSaved, onRefresh, onBusyChange }: { taskId: string; body: QuestionBody; canAnswer: boolean; onClose: () => void; onSaved: () => Promise<void>; onRefresh: () => Promise<void>; onBusyChange: (busy: boolean) => void }) {
  const key = `${taskId}:${body.interactionId}:${body.version}`;
  const [answers, setAnswers] = useState<Answer[]>(() => drafts.get(key).answers);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const [saved, setSaved] = useState(false);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const update = (questionKey: string, patch: Partial<Answer>) => {
    const next = body.questions.map((q) => { const a = answers.find((a) => a.questionKey === q.key) ?? { questionKey: q.key, selectedOptionKeys: [], text: '' }; return q.key === questionKey ? { ...a, ...patch } : a; });
    setAnswers(next); drafts.set(key, { ...drafts.get(key), answers: next });
  };
  return <Form layout="vertical" onFinish={async () => {
    if (!canAnswer || pending.current || saved) return;
    const validation = validateAnswers(body.questions, answers);
    if (validation) { setError(validation); return; }
    const idempotencyKey = drafts.submission(key, JSON.stringify(answers), () => crypto.randomUUID());
    pending.current = true; setBusy(true); onBusyChange(true); setError('');
    try {
      await respondToInteraction(taskId, body.interactionId, { expectedVersion: body.version, idempotencyKey, answers });
      if (!alive.current) return;
      setSaved(true); drafts.clear(key);
      await onSaved();
    } catch (e) { if (alive.current) { setError(conversationFeedback(e, '回答提交')); void onRefresh().catch(() => undefined); } }
    finally { pending.current = false; onBusyChange(false); if (alive.current) setBusy(false); }
  }}>
    {!canAnswer && !saved && <Typography.Paragraph>当前问题已变化或暂不可回答。你可以保留或复制草稿。</Typography.Paragraph>}
    {body.questions.map((q) => {
      const answer = answers.find((a) => a.questionKey === q.key);
      return <Form.Item key={q.key} label={q.title} required={q.required} extra={q.description}>
        {q.kind === 'SINGLE_CHOICE' && <Radio.Group aria-label={q.title} disabled={busy || saved || !canAnswer} value={answer?.selectedOptionKeys[0]} options={q.options.map((o) => ({ value: o.key, label: o.label }))} onChange={(e) => update(q.key, { selectedOptionKeys: [e.target.value] })} />}
        {q.kind === 'MULTIPLE_CHOICE' && <Checkbox.Group aria-label={q.title} disabled={busy || saved || !canAnswer} value={answer?.selectedOptionKeys ?? []} options={q.options.map((o) => ({ value: o.key, label: o.label }))} onChange={(values) => update(q.key, { selectedOptionKeys: values as string[] })} />}
        {q.allowFreeText && q.kind !== 'TEXT' && <Typography.Paragraph className="workforce-chat-free-text-label">补充说明（可选）</Typography.Paragraph>}
        {(q.kind === 'TEXT' || q.allowFreeText) && <Input.TextArea placeholder={q.kind === 'TEXT' ? '请输入回答…' : '请输入补充说明…'} aria-label={q.kind === 'TEXT' ? q.title : `${q.title}：补充说明`} readOnly={busy || saved || !canAnswer} value={answer?.text ?? ''} maxLength={20000} autoSize={{ minRows: 2, maxRows: 6 }} onChange={(e) => update(q.key, { text: e.target.value })} />}
      </Form.Item>;
    })}
    {error && <div role="alert">{error}</div>}
    {saved && <Typography.Paragraph role="status">回答已保存</Typography.Paragraph>}
    <Space><Button htmlType="submit" type="primary" loading={busy} disabled={!canAnswer || saved}>提交回答</Button><Button type="link" disabled={busy} onClick={onClose}>返回对话</Button></Space>
  </Form>;
}
function DeliveryStatus({ value, closed = false }: { value?: string; closed?: boolean }) {
  const labels: Record<string, string> = { SAVED: closed ? '已保存，未送达' : '已保存，等待送达', DELIVERED: '已送达' };
  return value && value !== 'NONE' ? <Typography.Text type="secondary">{labels[value]}</Typography.Text> : null;
}
