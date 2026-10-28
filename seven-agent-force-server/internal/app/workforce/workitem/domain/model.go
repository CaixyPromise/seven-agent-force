// Package domain owns the serial development workflow, not Agent execution.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	transport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
	tf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	"io"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound = errors.New("WORK_ITEM_NOT_FOUND")
	ErrConflict = errors.New("WORK_ITEM_VERSION_CONFLICT")
	ErrInvalid  = errors.New("WORK_ITEM_INPUT_INVALID")
	ErrPlan     = errors.New("WORK_ITEM_PLAN_INVALID")
)

const MaxPlanBytes = 128 * 1024

type Selection struct {
	ModelSelection          tf.ModelSelection `json:"modelSelection"`
	RuntimeReleaseID        int64             `json:"runtimeReleaseId,string"`
	ExecutionTimeoutSeconds int               `json:"executionTimeoutSeconds,omitempty"`
}

func (s Selection) TaskRequest() tf.ProjectTaskRequest {
	return tf.ProjectTaskRequest{ModelSelection: s.ModelSelection, RuntimeReleaseID: s.RuntimeReleaseID, ExecutionTimeoutSeconds: s.ExecutionTimeoutSeconds}
}

type Create struct {
	Name                    string               `json:"name"`
	Instruction             string               `json:"instruction"`
	PlanSelection           Selection            `json:"planSelection"`
	ImplementationSelection Selection            `json:"implementationSelection"`
	ReferenceAssetIDs       transport.Int64Slice `json:"referenceAssetIds,omitempty"`
	DeliveryPreference      string               `json:"deliveryPreference,omitempty"`
	UsagePolicy             struct {
		Mode string `json:"mode"`
	} `json:"usagePolicy,omitempty"`
}

func (c *Create) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	c.Instruction = strings.TrimSpace(c.Instruction)
	if !tf.ValidInstruction(c.Instruction) || len(c.Name) > 255 {
		return ErrInvalid
	}
	for _, s := range []Selection{c.PlanSelection, c.ImplementationSelection} {
		if s.ModelSelection.CredentialID < 1 || strings.TrimSpace(s.ModelSelection.Model) == "" || s.RuntimeReleaseID < 1 || s.ExecutionTimeoutSeconds < 0 {
			return ErrInvalid
		}
	}
	if c.UsagePolicy.Mode == "" {
		c.UsagePolicy.Mode = "OBSERVE"
	}
	if c.UsagePolicy.Mode != "OBSERVE" {
		return ErrInvalid
	}
	if c.DeliveryPreference == "" {
		c.DeliveryPreference = "PROJECT_DEFAULT"
	}
	if c.DeliveryPreference != "PROJECT_DEFAULT" && c.DeliveryPreference != "SAVE_ONLY" && c.DeliveryPreference != "ASK_AFTER_GATES" {
		return ErrInvalid
	}
	if c.Name == "" {
		r := []rune(c.Instruction)
		if len(r) > 80 {
			r = r[:80]
		}
		c.Name = string(r)
	}
	for _, id := range c.ReferenceAssetIDs {
		if id < 1 {
			return ErrInvalid
		}
	}
	return nil
}

type Step struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}
type Question struct {
	Key        string `json:"key"`
	Question   string `json:"question"`
	Blocking   bool   `json:"blocking"`
	Resolution string `json:"resolution,omitempty"`
}
type AffectedArea struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Plan struct {
	Schema             string         `json:"schema"`
	Title              string         `json:"title"`
	Summary            string         `json:"summary"`
	Goals              []string       `json:"goals"`
	NonGoals           []string       `json:"nonGoals"`
	Approach           string         `json:"approach"`
	AffectedAreas      []AffectedArea `json:"affectedAreas"`
	Steps              []Step         `json:"steps"`
	AcceptanceCriteria []string       `json:"acceptanceCriteria"`
	Risks              []string       `json:"risks"`
	OpenQuestions      []Question     `json:"openQuestions"`
	Markdown           string         `json:"markdown"`
	// An Agent may propose source metadata, but the platform's frozen Source is
	// the sole authority and the proposal is never included in the revision.
	CandidateSource json.RawMessage `json:"source,omitempty"`
}

func ParsePlan(body []byte) (Plan, error) {
	var p Plan
	if len(body) == 0 || len(body) > MaxPlanBytes || !utf8.Valid(body) {
		return p, ErrPlan
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&p) != nil || decoder.Decode(new(any)) != io.EOF {
		return p, ErrPlan
	}
	p.CandidateSource = nil
	if p.Schema != "seven.force.development-plan/v1" || strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Approach) == "" || len(p.Goals) == 0 || len(p.AcceptanceCriteria) == 0 || len(p.Steps) == 0 {
		return p, ErrPlan
	}
	for _, s := range append(append([]string{}, p.Goals...), p.AcceptanceCriteria...) {
		if strings.TrimSpace(s) == "" {
			return p, ErrPlan
		}
	}
	seen := map[string]bool{}
	for _, step := range p.Steps {
		if strings.TrimSpace(step.Key) == "" || strings.TrimSpace(step.Description) == "" || seen[step.Key] {
			return p, ErrPlan
		}
		seen[step.Key] = true
	}
	questions := map[string]bool{}
	for _, q := range p.OpenQuestions {
		if strings.TrimSpace(q.Key) == "" || strings.TrimSpace(q.Question) == "" || questions[q.Key] {
			return p, ErrPlan
		}
		questions[q.Key] = true
	}
	for _, area := range p.AffectedAreas {
		if strings.TrimSpace(area.Path) == "" || strings.TrimSpace(area.Reason) == "" {
			return p, ErrPlan
		}
	}
	return p, nil
}
func (p Plan) HasBlockingQuestions() bool {
	for _, q := range p.OpenQuestions {
		if q.Blocking && strings.TrimSpace(q.Resolution) == "" {
			return true
		}
	}
	return false
}

type Revision struct {
	ID               int64          `json:"id,string"`
	Number           int            `json:"revision"`
	TaskID           int64          `json:"taskId,string"`
	OutputID         int64          `json:"outputId,string"`
	ArtifactSHA256   string         `json:"artifactSha256"`
	Plan             Plan           `json:"plan"`
	Source           tf.StageSource `json:"source"`
	Digest           string         `json:"contentDigest"`
	ParentRevisionID int64          `json:"parentRevisionId,string,omitempty"`
	Origin           string         `json:"origin,omitempty"`
}

func (r Revision) DigestBody() ([]byte, string) {
	b, _ := json.Marshal(struct {
		Plan   Plan           `json:"plan"`
		Source tf.StageSource `json:"source"`
	}{r.Plan, r.Source})
	return b, Hash(b)
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Run struct {
	ID     int64  `json:"id,string"`
	Stage  string `json:"stage"`
	Number int    `json:"runNumber"`
	TaskID int64  `json:"taskId,string"`
	Status string `json:"status"`
}
type Item struct {
	ID                int64           `json:"id,string"`
	TenantID          int64           `json:"-"`
	ProjectID         int64           `json:"projectId,string"`
	ProjectVersionID  int64           `json:"projectVersionId,string"`
	Creator           int64           `json:"createdBy,string"`
	Name              string          `json:"name"`
	Instruction       string          `json:"instruction"`
	Status            string          `json:"status"`
	Version           int64           `json:"version,string"`
	Source            tf.StageSource  `json:"source"`
	Request           Create          `json:"-"`
	CurrentPlan       *Revision       `json:"currentPlan,omitempty"`
	Runs              []Run           `json:"runs"`
	Usage             json.RawMessage `json:"usage"`
	AvailableActions  []string        `json:"availableActions"`
	ErrorCode         string          `json:"reasonCode,omitempty"`
	CreatedAt         string          `json:"createdAt"`
	CancelRequestedBy int64           `json:"cancelRequestedBy,string,omitempty"`
}

func (i *Item) Actions() {
	i.AvailableActions = []string{}
	if i.Status == "AWAITING_PLAN_APPROVAL" && i.CurrentPlan != nil && !i.CurrentPlan.Plan.HasBlockingQuestions() {
		i.AvailableActions = append(i.AvailableActions, "APPROVE_PLAN")
	}
	if i.Status == "AWAITING_PLAN_APPROVAL" && i.CurrentPlan != nil {
		i.AvailableActions = append(i.AvailableActions, "EDIT_PLAN", "REVISE_PLAN")
	}
	if i.Status == "NEEDS_ATTENTION" && len(i.Runs) > 0 && i.Runs[len(i.Runs)-1].Stage == "PLAN" {
		i.AvailableActions = append(i.AvailableActions, "REVISE_PLAN")
	}
	if i.Status != "CANCELLED" && i.Status != "CANCELLING" && i.Status != "SUCCEEDED" {
		i.AvailableActions = append(i.AvailableActions, "CANCEL")
	}
}
func (i Item) CanApprove(version, id int64, digest string) error {
	if i.Version != version || i.Status != "AWAITING_PLAN_APPROVAL" || i.CurrentPlan == nil || i.CurrentPlan.ID != id || i.CurrentPlan.Digest != digest || i.CurrentPlan.Plan.HasBlockingQuestions() {
		return ErrConflict
	}
	return nil
}
