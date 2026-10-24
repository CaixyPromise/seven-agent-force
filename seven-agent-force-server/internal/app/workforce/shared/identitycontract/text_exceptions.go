// Package identitycontract owns the explicit exceptions to Workforce's
// BIGINT internal database identity rule. Entries here are protocol, external
// system, security-token, or process-session identities; they are not internal
// entity or relation keys.
package identitycontract

var textIdentityExceptions = map[string]string{
	"workforce_task_attempts.container_id":                              "Docker container identity",
	"workforce_task_attempts.worker_instance_id":                        "worker process-session identity",
	"workforce_task_attempts.host_boot_id":                              "machine boot identity",
	"workforce_harness_sessions.provider_session_id":                    "provider session identity",
	"workforce_model_invocations.protocol_invocation_key":               "proxy protocol idempotency identity",
	"workforce_model_invocations.worker_instance_id":                    "worker process-session identity",
	"workforce_model_invocations.host_boot_id":                          "machine boot identity",
	"workforce_model_invocations.upstream_request_id":                   "model upstream request identity",
	"workforce_workers.current_instance_id":                             "worker process-session identity",
	"workforce_worker_sessions.instance_id":                             "worker process-session identity",
	"workforce_worker_sessions.host_boot_id":                            "machine boot identity",
	"workforce_worker_images.instance_id":                               "worker process-session identity",
	"workforce_task_assignments.worker_instance_id":                     "worker process-session identity",
	"workforce_attempt_runtimes.worker_instance_id":                     "worker process-session identity",
	"workforce_attempt_runtimes.host_boot_id":                           "machine boot identity",
	"workforce_attempt_runtimes.container_id":                           "Docker container identity",
	"workforce_attempt_runtimes.provider_proxy_container_id":            "Docker container identity",
	"workforce_attempt_runtimes.pending_worker_instance_id":             "worker process-session identity",
	"workforce_runtime_reconciliation_runs.old_instance_id":             "worker process-session identity",
	"workforce_runtime_reconciliation_runs.new_instance_id":             "worker process-session identity",
	"workforce_runtime_reconciliation_runs.host_boot_id":                "machine boot identity",
	"workforce_finalization_runs.worker_instance_id":                    "worker process-session identity",
	"workforce_finalization_runs.host_boot_id":                          "machine boot identity",
	"workforce_artifact_upload_attempts.host_boot_id":                   "machine boot identity",
	"workforce_review_runs.worker_instance_id":                          "worker process-session identity",
	"workforce_review_runs.host_boot_id":                                "machine boot identity",
	"workforce_review_runs.runtime_container_id":                        "Docker container identity",
	"workforce_review_run_attempts.worker_instance_id":                  "worker process-session identity",
	"workforce_review_run_attempts.host_boot_id":                        "machine boot identity",
	"workforce_review_run_attempts.container_id":                        "Docker container identity",
	"workforce_attempt_facts.fact_id":                                   "Supervisor journal fact identity",
	"workforce_result_processor_fixture_runs.worker_instance_id":        "worker process-session identity",
	"workforce_continuation_checkpoints.encryption_key_id":              "external key-management identity",
	"workforce_continuation_restore_runs.worker_instance_id":            "worker process-session identity",
	"workforce_continuation_restore_runs.host_boot_id":                  "machine boot identity",
	"workforce_result_materialization_runs.worker_instance_id":          "worker process-session identity",
	"workforce_result_materialization_runs.host_boot_id":                "machine boot identity",
	"workforce_source_ingestion_runs.worker_instance_id":                "worker process-session identity",
	"workforce_source_ingestion_runs.host_boot_id":                      "machine boot identity",
	"workforce_pull_request_publications.provider_review_id":            "Git provider review identity",
	"workforce_delivery_attempts.worker_instance_id":                    "worker process-session identity",
	"workforce_delivery_attempts.host_boot_id":                          "machine boot identity",
	"workforce_integration_api_credentials.credential_locator":          "API key security-token locator",
	"workforce_public_events.correlation_id":                            "external system correlation identity",
	"workforce_tasks.external_correlation_id":                           "external system correlation identity",
	"workforce_webhook_deliveries.lease_owner":                          "webhook sender process-lease identity",
	"workforce_task_notification_records.notification_receipt_identity": "NotificationClient opaque receipt identity",
	"workforce_task_notification_records.idempotency_identity":          "cross-module notification idempotency protocol identity",
	"workforce_task_notification_records.lease_token":                   "short-lived publication claim security token",
	"workforce_execution_operation_attempts.container_id":               "Docker container identity",
	"workforce_execution_operations.native_tool_call_id":                "Adapter-native external tool call identity",
	"workforce_delivery_intents.native_tool_call_id":                    "Adapter-native external Git delivery tool-call identity",
}

// IsTextIdentityException reports whether a TEXT column is an explicitly
// governed non-entity identity exception.
func IsTextIdentityException(column string) bool {
	_, ok := textIdentityExceptions[column]
	return ok
}

// TextIdentityExceptions returns a defensive copy so callers cannot mutate the
// repository-wide identity contract.
func TextIdentityExceptions() map[string]string {
	result := make(map[string]string, len(textIdentityExceptions))
	for column, reason := range textIdentityExceptions {
		result[column] = reason
	}
	return result
}
