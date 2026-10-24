package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
)

const (
	reasonRevisionLimit      = "INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED"
	reasonNoMaterialProgress = "INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS"
	reasonCheckpointMissing  = "INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE"
	reasonRevisionConflict   = "INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT"
	// A human-authorized correction grants exactly one Turn and does not reopen
	// the automatic loop. Its effective automatic budget is exhausted by policy.
	reasonHumanCycleRequires = reasonRevisionLimit
)

type progressFacts struct {
	AutomaticRevisionNumber        int
	BlockingFindingCount           int
	ActionableFindingDigest        string
	DeterministicTestSummaryDigest string
	DeterministicFailureCount      int
	ProgressSignature              string
	NoProgressStreak               int
	CurrentRevisionKind            string
}

func revisionStopReason(facts progressFacts, automaticLimit int) string {
	if facts.CurrentRevisionKind == continuationfacade.RevisionHumanAuthorized {
		return reasonHumanCycleRequires
	}
	if facts.AutomaticRevisionNumber >= automaticLimit {
		return reasonRevisionLimit
	}
	if facts.NoProgressStreak >= 2 {
		return reasonNoMaterialProgress
	}
	return ""
}

func (s *Store) calculateProgress(ctx context.Context, tx pgx.Tx, runID int64, report domain.CanonicalReport) (progressFacts, error) {
	var taskID, turnID, attemptID int64
	var resultTree string
	if err := tx.QueryRow(ctx, `SELECT task_id,task_turn_id,attempt_id,result_tree FROM workforce_review_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&taskID, &turnID, &attemptID, &resultTree); err != nil {
		return progressFacts{}, err
	}
	var facts progressFacts
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE review_revision_kind='AUTOMATIC') FROM workforce_task_turns WHERE task_id=$1`, taskID).Scan(&facts.AutomaticRevisionNumber); err != nil {
		return progressFacts{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(review_revision_kind,'') FROM workforce_task_turns WHERE id=$1`, turnID).Scan(&facts.CurrentRevisionKind); err != nil {
		return progressFacts{}, err
	}
	currentSet := actionableSet(report)
	facts.BlockingFindingCount = blockingCount(report)
	facts.ActionableFindingDigest = digestStrings(currentSet)

	type checkFact struct {
		Ordinal int    `json:"ordinal"`
		Digest  string `json:"commandDigest"`
		Status  string `json:"status"`
		Exit    int    `json:"exitCode"`
	}
	rows, err := tx.Query(ctx, `SELECT ordinal,command_digest,status,COALESCE(exit_code,-1) FROM workforce_check_runs WHERE task_id=$1 AND attempt_id=$2 ORDER BY ordinal`, taskID, attemptID)
	if err != nil {
		return progressFacts{}, err
	}
	checks := make([]checkFact, 0)
	for rows.Next() {
		var item checkFact
		if err = rows.Scan(&item.Ordinal, &item.Digest, &item.Status, &item.Exit); err != nil {
			rows.Close()
			return progressFacts{}, err
		}
		if item.Status != "SUCCEEDED" || item.Exit != 0 {
			facts.DeterministicFailureCount++
		}
		checks = append(checks, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return progressFacts{}, err
	}
	encodedChecks, _ := json.Marshal(checks)
	facts.DeterministicTestSummaryDigest = digestBytes(encodedChecks)
	facts.ProgressSignature = digestBytes([]byte(resultTree + "\x00" + facts.ActionableFindingDigest + "\x00" + facts.DeterministicTestSummaryDigest))

	var previousReport []byte
	var previousTree string
	var previousBlocking, previousFailures, previousStreak int
	err = tx.QueryRow(ctx, `SELECT canonical_report_json,result_tree,blocking_finding_count,deterministic_failure_count,no_progress_streak
FROM workforce_review_runs WHERE task_id=$1 AND id<$2 AND canonical_report_json IS NOT NULL
ORDER BY id DESC LIMIT 1`, taskID, runID).Scan(&previousReport, &previousTree, &previousBlocking, &previousFailures, &previousStreak)
	if err == pgx.ErrNoRows {
		return facts, nil
	}
	if err != nil {
		return progressFacts{}, err
	}
	var previous domain.CanonicalReport
	if json.Unmarshal(previousReport, &previous) != nil {
		return progressFacts{}, domain.ErrInvalidReport
	}
	previousSet := actionableSet(previous)
	material := hasMaterialProgress(facts.BlockingFindingCount, facts.DeterministicFailureCount, currentSet, resultTree, score(report), previousBlocking, previousFailures, previousSet, previousTree, score(previous))
	if material {
		facts.NoProgressStreak = 0
	} else {
		facts.NoProgressStreak = previousStreak + 1
	}
	return facts, nil
}

func hasMaterialProgress(currentBlocking, currentFailures int, currentSet []string, currentTree string, currentScore int, previousBlocking, previousFailures int, previousSet []string, previousTree string, previousScore int) bool {
	if currentBlocking < previousBlocking || currentFailures < previousFailures || strictSubset(currentSet, previousSet) {
		return true
	}
	return currentBlocking <= previousBlocking && currentFailures <= previousFailures && currentTree != previousTree && currentScore > previousScore
}

func persistProgress(ctx context.Context, tx pgx.Tx, runID int64, facts progressFacts) error {
	_, err := tx.Exec(ctx, `UPDATE workforce_review_runs SET automatic_revision_number=$2,blocking_finding_count=$3,actionable_finding_digest=$4,deterministic_test_summary_digest=$5,deterministic_failure_count=$6,progress_signature=$7,no_progress_streak=$8,updated_at=CURRENT_TIMESTAMP WHERE id=$1`,
		runID, facts.AutomaticRevisionNumber, facts.BlockingFindingCount, facts.ActionableFindingDigest, facts.DeterministicTestSummaryDigest, facts.DeterministicFailureCount, facts.ProgressSignature, facts.NoProgressStreak)
	return err
}

func actionableSet(report domain.CanonicalReport) []string {
	items := make([]string, 0, len(report.Findings))
	for _, finding := range report.Findings {
		if finding.Severity != "WARNING" && finding.Severity != "BLOCKING" {
			continue
		}
		items = append(items, fmt.Sprintf("%s\x00%s\x00%s\x00%d", finding.Severity, finding.Category, finding.Evidence.Path, finding.Evidence.Line))
	}
	sort.Strings(items)
	return compactStrings(items)
}

func strictSubset(current, previous []string) bool {
	if len(current) >= len(previous) {
		return false
	}
	want := make(map[string]struct{}, len(previous))
	for _, item := range previous {
		want[item] = struct{}{}
	}
	for _, item := range current {
		if _, ok := want[item]; !ok {
			return false
		}
	}
	return true
}

func blockingCount(report domain.CanonicalReport) int {
	count := 0
	for _, finding := range report.Findings {
		if finding.Severity == "BLOCKING" {
			count++
		}
	}
	return count
}

func score(report domain.CanonicalReport) int {
	if report.Score == nil {
		return -1
	}
	return *report.Score
}

func compactStrings(items []string) []string {
	result := items[:0]
	for _, item := range items {
		if len(result) == 0 || result[len(result)-1] != item {
			result = append(result, item)
		}
	}
	return result
}

func digestStrings(items []string) string { return digestBytes([]byte(strings.Join(items, "\n"))) }

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
