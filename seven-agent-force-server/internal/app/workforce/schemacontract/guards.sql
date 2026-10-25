-- Bring legacy validation and readiness diagnostics to the current module contracts.
DO $guards$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_harness_releases'::regclass AND conname='workforce_harness_releases_hook_events_ck') THEN
        ALTER TABLE public.workforce_harness_releases ADD CONSTRAINT workforce_harness_releases_hook_events_ck CHECK (jsonb_typeof(hook_events_json) = 'array'::text) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_harness_releases VALIDATE CONSTRAINT workforce_harness_releases_hook_events_ck;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_harness_releases'::regclass AND conname='workforce_harness_releases_manifest_ck') THEN
        ALTER TABLE public.workforce_harness_releases ADD CONSTRAINT workforce_harness_releases_manifest_ck CHECK (jsonb_typeof(manifest_json) = 'object'::text) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_harness_releases VALIDATE CONSTRAINT workforce_harness_releases_manifest_ck;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_harness_releases'::regclass AND conname='workforce_harness_releases_provider_protocols_ck') THEN
        ALTER TABLE public.workforce_harness_releases ADD CONSTRAINT workforce_harness_releases_provider_protocols_ck CHECK (jsonb_typeof(provider_protocols_json) = 'array'::text) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_harness_releases VALIDATE CONSTRAINT workforce_harness_releases_provider_protocols_ck;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_credentials'::regclass AND conname='workforce_model_credentials_models_json_check') THEN
        ALTER TABLE public.workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_models_json_check CHECK (jsonb_typeof(models_json) = 'array'::text) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_credentials VALIDATE CONSTRAINT workforce_model_credentials_models_json_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_credentials'::regclass AND conname='workforce_model_credentials_models_json_check1') THEN
        ALTER TABLE public.workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_models_json_check1 CHECK (jsonb_array_length(models_json) <= 5000) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_credentials VALIDATE CONSTRAINT workforce_model_credentials_models_json_check1;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_credentials'::regclass AND conname='workforce_model_credentials_secret_version_check') THEN
        ALTER TABLE public.workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_secret_version_check CHECK (secret_version > 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_credentials VALIDATE CONSTRAINT workforce_model_credentials_secret_version_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_credentials'::regclass AND conname='workforce_model_credentials_version_check') THEN
        ALTER TABLE public.workforce_model_credentials ADD CONSTRAINT workforce_model_credentials_version_check CHECK (version > 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_credentials VALIDATE CONSTRAINT workforce_model_credentials_version_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_cache_creation_input_tokens_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_cache_creation_input_tokens_check CHECK (cache_creation_input_tokens IS NULL OR cache_creation_input_tokens >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_cache_creation_input_tokens_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_cached_input_tokens_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_cached_input_tokens_check CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_cached_input_tokens_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_credential_version_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_credential_version_check CHECK (credential_version > 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_credential_version_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_duration_milliseconds_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_duration_milliseconds_check CHECK (duration_milliseconds IS NULL OR duration_milliseconds >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_duration_milliseconds_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_input_tokens_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_input_tokens_check CHECK (input_tokens IS NULL OR input_tokens >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_input_tokens_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_nonce_sha256_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_nonce_sha256_check CHECK (octet_length(nonce_sha256) = 32) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_nonce_sha256_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_output_tokens_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_output_tokens_check CHECK (output_tokens IS NULL OR output_tokens >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_output_tokens_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_reasoning_output_tokens_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_reasoning_output_tokens_check CHECK (reasoning_output_tokens IS NULL OR reasoning_output_tokens >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_reasoning_output_tokens_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_request_body_sha256_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_request_body_sha256_check CHECK (octet_length(request_body_sha256) = 32) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_request_body_sha256_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_total_tokens_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_total_tokens_check CHECK (total_tokens IS NULL OR total_tokens >= 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_total_tokens_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_model_invocations'::regclass AND conname='workforce_model_invocations_upstream_status_code_check') THEN
        ALTER TABLE public.workforce_model_invocations ADD CONSTRAINT workforce_model_invocations_upstream_status_code_check CHECK (upstream_status_code IS NULL OR upstream_status_code >= 100 AND upstream_status_code <= 599) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_model_invocations VALIDATE CONSTRAINT workforce_model_invocations_upstream_status_code_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_review_run_attempts'::regclass AND conname='workforce_review_run_attempts_credential_version_check') THEN
        ALTER TABLE public.workforce_review_run_attempts ADD CONSTRAINT workforce_review_run_attempts_credential_version_check CHECK (credential_version > 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_review_run_attempts VALIDATE CONSTRAINT workforce_review_run_attempts_credential_version_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_review_run_attempts'::regclass AND conname='workforce_review_run_attempts_model_protocol_check') THEN
        ALTER TABLE public.workforce_review_run_attempts ADD CONSTRAINT workforce_review_run_attempts_model_protocol_check CHECK (model_protocol::text = ANY (ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying]::text[])) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_review_run_attempts VALIDATE CONSTRAINT workforce_review_run_attempts_model_protocol_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_task_assignments'::regclass AND conname='workforce_task_assignments_credential_version_check') THEN
        ALTER TABLE public.workforce_task_assignments ADD CONSTRAINT workforce_task_assignments_credential_version_check CHECK (credential_version > 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_task_assignments VALIDATE CONSTRAINT workforce_task_assignments_credential_version_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_task_assignments'::regclass AND conname='workforce_task_assignments_hook_configuration_digest_check') THEN
        ALTER TABLE public.workforce_task_assignments ADD CONSTRAINT workforce_task_assignments_hook_configuration_digest_check CHECK (hook_configuration_digest::text = ''::text OR hook_configuration_digest::text ~ '^[0-9a-f]{64}$'::text) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_task_assignments VALIDATE CONSTRAINT workforce_task_assignments_hook_configuration_digest_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_task_assignments'::regclass AND conname='workforce_task_assignments_model_protocol_check') THEN
        ALTER TABLE public.workforce_task_assignments ADD CONSTRAINT workforce_task_assignments_model_protocol_check CHECK (model_protocol::text = ANY (ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying]::text[])) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_task_assignments VALIDATE CONSTRAINT workforce_task_assignments_model_protocol_check;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_tasks'::regclass AND conname='workforce_tasks_model_selection_ck') THEN
        ALTER TABLE public.workforce_tasks ADD CONSTRAINT workforce_tasks_model_selection_ck CHECK (model_credential_id IS NULL AND primary_model IS NULL OR model_credential_id > 0 AND btrim(primary_model::text) <> ''::text) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_tasks VALIDATE CONSTRAINT workforce_tasks_model_selection_ck;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.workforce_tasks'::regclass AND conname='workforce_tasks_selected_runtime_release_id_ck') THEN
        ALTER TABLE public.workforce_tasks ADD CONSTRAINT workforce_tasks_selected_runtime_release_id_ck CHECK (selected_runtime_release_id IS NULL OR selected_runtime_release_id > 0) NOT VALID;
    END IF;
    ALTER TABLE public.workforce_tasks VALIDATE CONSTRAINT workforce_tasks_selected_runtime_release_id_ck;
    ALTER TABLE public.workforce_review_run_attempts DROP CONSTRAINT IF EXISTS workforce_review_run_attempts_provider_protocol_check;
END
$guards$;
CREATE INDEX IF NOT EXISTS workforce_model_invocations_credential_idx ON public.workforce_model_invocations USING btree (tenant_id, model_credential_id, started_at DESC);
CREATE INDEX IF NOT EXISTS workforce_model_invocations_task_idx ON public.workforce_model_invocations USING btree (tenant_id, task_id, started_at DESC);
CREATE INDEX IF NOT EXISTS workforce_model_credentials_tenant_status_idx ON public.workforce_model_credentials USING btree (tenant_id, status, name);
CREATE OR REPLACE FUNCTION public.workforce_assert_project_hard_cut_ready()
 RETURNS void
 LANGUAGE plpgsql
 SET search_path TO 'pg_catalog', 'public'
AS $function$
DECLARE
  relation_name TEXT;
  has_business_rows BOOLEAN;
BEGIN
  FOREACH relation_name IN ARRAY ARRAY[
    'workforce_repository_connections',
    'workforce_repository_access_bindings',
    'workforce_repository_credential_events',
    'workforce_repository_lifecycle_events',
    'workforce_hook_bundles',
    'workforce_hook_bundle_releases',
    'workforce_hook_bindings',
    'workforce_hook_execution_profiles',
    'workforce_golden_verification_runs',
    'workforce_integrated_verification_runs'
  ] LOOP
    IF pg_catalog.to_regclass(pg_catalog.format('public.%I',relation_name)) IS NULL THEN
      CONTINUE;
    END IF;
    EXECUTE pg_catalog.format(
      'SELECT EXISTS (SELECT 1 FROM public.%I LIMIT 1)',
      relation_name
    ) INTO has_business_rows;
    IF has_business_rows THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'PROJECT_REPOSITORY_HARD_CUT_RESET_REQUIRED';
    END IF;
  END LOOP;

  FOREACH relation_name IN ARRAY ARRAY[
    'workforce_tasks',
    'workforce_task_attempts',
    'workforce_harness_sessions',
    'workforce_completion_proposals',
    'workforce_task_events',
    'workforce_artifacts',
    'workforce_check_runs',
    'workforce_model_invocations',
    'workforce_task_execution_snapshots',
    'workforce_review_runs',
    'workforce_delivery_requests'
  ] LOOP
    IF pg_catalog.to_regclass(pg_catalog.format('public.%I',relation_name)) IS NULL THEN
      CONTINUE;
    END IF;
    EXECUTE pg_catalog.format(
      'SELECT EXISTS (SELECT 1 FROM public.%I LIMIT 1)',
      relation_name
    ) INTO has_business_rows;
    IF has_business_rows THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'PROJECT_HARD_CUT_RESET_REQUIRED';
    END IF;
  END LOOP;
END
$function$
;
