import type { Answer } from '@/api/conversationContract';

export type ConversationDraft = { text: string; answers: Answer[]; idempotencyKey?: string; fingerprint?: string };
/** Memory only. Scope changes erase drafts, including tasks no longer mounted. */
export class ConversationDrafts {
  private scope = '';
  private drafts = new Map<string, ConversationDraft>();
  setScope(scope: string) { if (scope !== this.scope) { this.drafts.clear(); this.scope = scope; } }
  get(key: string): ConversationDraft { return this.drafts.get(key) ?? { text: '', answers: [] }; }
  set(key: string, value: ConversationDraft) { this.drafts.set(key, value); }
  clear(key: string) { this.drafts.delete(key); }
  submission(key: string, fingerprint: string, newKey: () => string): string {
    const draft = this.get(key);
    if (draft.fingerprint === fingerprint && draft.idempotencyKey) return draft.idempotencyKey;
    const idempotencyKey = newKey();
    this.set(key, { ...draft, fingerprint, idempotencyKey });
    return idempotencyKey;
  }
}
