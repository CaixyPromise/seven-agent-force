package infrastructure

// FactProducerSchemaSQL is applied only by explicit unpublished-schema alignment,
// never at normal startup. It preserves fact IDs, sequences, payload hashes and
// events. Historical journal rows are recognized by exact persisted journal
// identity and the bounded safe event set, not by a newly submitted claim.
const FactProducerSchemaSQL = `
ALTER TABLE workforce_attempt_facts ADD COLUMN IF NOT EXISTS producer_source TEXT NOT NULL DEFAULT 'WORKER';
UPDATE workforce_attempt_facts SET producer_source='SUPERVISOR_JOURNAL'
 WHERE producer_source='WORKER' AND fact_id='jrn_'||attempt_id::text||'_'||producer_sequence::text
 AND event_type IN ('hook.started','hook.allow','hook.deny','hook.failed','owner.adopted','watchdog.expired','runtime.interrupted');
ALTER TABLE workforce_attempt_facts DROP CONSTRAINT IF EXISTS workforce_attempt_facts_producer_source_check;
ALTER TABLE workforce_attempt_facts ADD CONSTRAINT workforce_attempt_facts_producer_source_check CHECK(producer_source IN ('WORKER','SUPERVISOR_JOURNAL'));
ALTER TABLE workforce_attempt_facts DROP CONSTRAINT IF EXISTS workforce_attempt_facts_attempt_id_producer_sequence_key;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_attempt_facts_source_sequence_uidx ON workforce_attempt_facts(attempt_id,producer_source,producer_sequence);
`
