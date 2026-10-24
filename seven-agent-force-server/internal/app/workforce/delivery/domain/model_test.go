package domain

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPreparedPushExpiryRecoveryAdmission(t *testing.T) {
	now := time.Now().UTC()
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	for _, c := range []struct {
		action, request, attempt, phase string
		expiry                          *time.Time
		want                            bool
	}{
		{ActionGitPush, "CONFIRMING", "RUNNING", "PREPARED", &past, true},
		{ActionGitPush, "CONFIRMING", "RUNNING", "PREPARED", &future, false},
		{ActionGitPush, "CONFIRMING", "RUNNING", "PREPARED", &now, false},
		{ActionGitPush, "CONFIRMING", "RUNNING", "PREPARED", nil, false},
		{ActionGitPush, "CONFIRMING", "PENDING", "PREPARED", &past, false},
		{ActionGitPush, "CONFIRMING", "RUNNING", "PREPARING", &past, false},
		{ActionGitPush, "SUCCEEDED", "RUNNING", "PREPARED", &past, false},
		{ActionCustomDelivery, "CONFIRMING", "RUNNING", "PREPARED", &past, false},
	} {
		if got := CanReconcileExpiredPreparedPush(c.action, c.request, c.attempt, c.phase, c.expiry, now); got != c.want {
			t.Fatalf("admission=%v want=%v for %+v", got, c.want, c)
		}
	}
}

func TestDeliveryCommitMatchesGitHashObject(t *testing.T) {
	t.Parallel()
	tree := strings.Repeat("1", 40)
	parent := strings.Repeat("2", 40)
	when := time.Unix(1700000000, 0).UTC()
	message := "Seven Agent Force result\n"
	got, err := ComputeDeliveryCommit(tree, parent, "Seven Bot", "bot@example.invalid", when, message)
	if err != nil {
		t.Fatal(err)
	}
	body := "tree " + tree + "\nparent " + parent + "\nauthor Seven Bot <bot@example.invalid> 1700000000 +0000\ncommitter Seven Bot <bot@example.invalid> 1700000000 +0000\n\n" + message
	command := exec.Command("git", "hash-object", "-t", "commit", "--stdin")
	command.Stdin = strings.NewReader(body)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimSpace(string(output)); got != want {
		t.Fatalf("commit=%s want=%s", got, want)
	}
}

func TestRemoteArbitrationNeverBlindlyRetriesUnknown(t *testing.T) {
	t.Parallel()
	expected := strings.Repeat("1", 40)
	delivery := strings.Repeat("2", 40)
	if got := ReconcileAfter(expected, delivery, "", false); got.Status != "OUTCOME_UNKNOWN" || got.SafeToRetry {
		t.Fatalf("unknown=%+v", got)
	}
	if got := ReconcileAfter(expected, delivery, delivery, true); got.Status != "SUCCEEDED" {
		t.Fatalf("success=%+v", got)
	}
	if got := ReconcileAfter(expected, delivery, expected, true); got.Status != "RETRY_WAIT" || !got.SafeToRetry {
		t.Fatalf("unchanged=%+v", got)
	}
	if got := ReconcileAfter(expected, delivery, strings.Repeat("3", 40), true); got.Status != "MANUAL_REVIEW" || got.SafeToRetry {
		t.Fatalf("unexpected=%+v", got)
	}
}

func TestPrepareRejectsProcessorControlledRemoteIdentity(t *testing.T) {
	t.Parallel()
	target := Target{ID: 1719730041, TenantID: 1001, Name: "local", Transport: TransportLocalBare, RepositoryPath: "/tmp/repo.git", CommitAuthorName: "Seven Bot", CommitAuthorEmail: "bot@example.invalid", Status: "ENABLED", ConfigDigest: strings.Repeat("a", 64)}
	input := RequestInput{TenantID: 1001, TaskID: 2610240832, TaskTurnID: 637649102, CheckpointID: 3433751987, ProcessorDigest: strings.Repeat("b", 64), ResultCommit: strings.Repeat("c", 40), ResultTree: strings.Repeat("d", 40), ExpectedBaseCommit: strings.Repeat("e", 40), Action: Action{Type: ActionGitPush, When: ApprovalAfter, TargetID: 1719730041, BaseBranch: "main", Branch: "seven/task-1"}, Target: target, TargetConfigDigest: target.ConfigDigest, CredentialVersion: "1", AuthorizationDigest: strings.Repeat("f", 64), Now: time.Unix(1700000000, 0)}
	prepared, err := Prepare(input)
	if err != nil || prepared.InitialStatus != "WAITING_APPROVAL" || len(prepared.DeliveryCommit) != 40 {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	input.Action.Branch = "main"
	if _, err = Prepare(input); err == nil {
		t.Fatal("protected/unallowed branch accepted")
	}
}

func TestBindActionToGitPlanUsesFrozenIdentityAndPriorDeliveryParent(t *testing.T) {
	t.Parallel()
	base := strings.Repeat("1", 40)
	previous := strings.Repeat("2", 40)
	plan := FrozenGitPlan{TargetID: 175719119, BaseBranch: "main", BaseCommit: base, DestinationBranch: "seven/task-t2", DeliveryMode: "NEW_BRANCH_AND_DRAFT_REVIEW", PreviousDeliveryCommit: previous}
	action, expected, err := BindActionToGitPlan(Action{Type: ActionGitPush, When: ApprovalAfter}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if action.TargetID != plan.TargetID || action.BaseBranch != plan.BaseBranch || action.Branch != plan.DestinationBranch || expected != previous {
		t.Fatalf("action=%+v expected=%s", action, expected)
	}
	_, _, err = BindActionToGitPlan(Action{Type: ActionGitPush, TargetID: 1754366601}, plan)
	if !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("redirect err=%v", err)
	}
}

func TestAuthorizationDigestFencesCredentialAndConnectionDrift(t *testing.T) {
	t.Parallel()
	baseline := AuthorizationDigest(strings.Repeat("a", 64), "PUSH_BRANCH", "system.git.push", "v1")
	if baseline == AuthorizationDigest(strings.Repeat("a", 64), "PUSH_BRANCH", "system.git.push", "v2") {
		t.Fatal("credential version drift retained authority")
	}
	if baseline == AuthorizationDigest(strings.Repeat("b", 64), "PUSH_BRANCH", "system.git.push", "v1") {
		t.Fatal("connection policy drift retained authority")
	}
	if baseline == AuthorizationDigest(strings.Repeat("a", 64), "CREATE_REVIEW", "system.git.push", "v1") {
		t.Fatal("capability separation retained authority")
	}
}

func TestTargetIdentityDigestAllowsCredentialRotationButRejectsRemoteDrift(t *testing.T) {
	t.Parallel()
	target := Target{
		ID: 175719119, TenantID: 1001, Name: "private", ProviderType: "GITEA",
		Transport: TransportSSH, Host: "git.example.test", Port: 2222,
		RepositoryPath: "org/private.git", WebRepositoryURL: "https://git.example.test/org/private",
		CredentialRef: "cfg:push-v1", CredentialVersion: "1", SSHKnownHostFingerprint: "SHA256:pin",
		EgressPolicyDigest: "egress-a", CommitAuthorName: "Seven Bot", CommitAuthorEmail: "bot@example.test", Status: "ENABLED",
	}
	first, err := ComputeTargetIdentityDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	target.CredentialRef, target.CredentialVersion = "cfg:push-v2", "2"
	second, err := ComputeTargetIdentityDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("credential rotation must not change static target identity")
	}
	target.RepositoryPath = "org/other.git"
	third, err := ComputeTargetIdentityDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("repository drift must change static target identity")
	}
}

func TestCanReauthorizeExhaustedPush(t *testing.T) {
	for _, c := range []struct {
		name, action, status, reason string
		count                        int
		active, want                 bool
	}{
		{"readback exhausted", ActionGitPush, "MANUAL_REVIEW", "DELIVERY_READBACK_RETRY_EXHAUSTED", 3, false, true},
		{"push exhausted", ActionGitPush, "FAILED", "DELIVERY_RETRY_EXHAUSTED", 3, false, true},
		{"active owner", ActionGitPush, "MANUAL_REVIEW", "DELIVERY_READBACK_RETRY_EXHAUSTED", 3, true, false},
		{"budget remains", ActionGitPush, "MANUAL_REVIEW", "DELIVERY_READBACK_RETRY_EXHAUSTED", 2, false, false},
		{"unexpected remote", ActionGitPush, "MANUAL_REVIEW", "REMOTE_REF_UNEXPECTED", 3, false, false},
		{"cancelled", ActionGitPush, "CANCELLED", "DELIVERY_RETRY_EXHAUSTED", 3, false, false},
		{"custom delivery", ActionCustomDelivery, "FAILED", "DELIVERY_RETRY_EXHAUSTED", 3, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := CanReauthorizeExhaustedPush(c.action, c.status, c.reason, c.count, c.active); got != c.want {
				t.Fatalf("got=%v want=%v", got, c.want)
			}
		})
	}
}
