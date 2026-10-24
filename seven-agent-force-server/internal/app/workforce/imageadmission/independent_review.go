package imageadmission

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	reviewimagefacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/imageadmission/facade"
	"github.com/jackc/pgx/v5"
)

// IndependentReviewImageCatalog is the Image Admission implementation of the
// small stable cross-module contract.  It has no Docker access: it consults
// only evidence that Worker/Image Admission already persisted.
type IndependentReviewImageCatalog struct{}

func NewIndependentReviewImageCatalog() reviewimagefacade.IndependentReviewImageResolver {
	return IndependentReviewImageCatalog{}
}

func (IndependentReviewImageCatalog) ListIndependentReviewImages(ctx context.Context, tx pgx.Tx, query, cursor string, limit int) (reviewimagefacade.IndependentReviewImagePage, error) {
	query = strings.TrimSpace(query)
	cursor = strings.TrimSpace(cursor)
	if len(query) > 128 || len(cursor) > 80 {
		return reviewimagefacade.IndependentReviewImagePage{}, reviewimagefacade.ErrIndependentReviewImageUnavailable
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	items := make([]reviewimagefacade.IndependentReviewImageOption, 0, limit+1)
	scanCursor := cursor
	for len(items) <= limit {
		rows, err := tx.Query(ctx, `SELECT DISTINCT ON (image_digest) image_digest,COALESCE(NULLIF(image_ref,''),image_digest),capabilities_json
FROM workforce_worker_images
WHERE role='attempt' AND contract_version='seven.worker-image/v1'
  AND admission_status='ADMITTED' AND presence_status='PRESENT'
  AND ($1='' OR lower(COALESCE(image_ref,'')) LIKE '%'||lower($1)||'%' OR image_digest LIKE '%'||lower($1)||'%')
  AND ($2='' OR image_digest>$2)
ORDER BY image_digest,image_ref LIMIT 100`, query, scanCursor)
		if err != nil {
			return reviewimagefacade.IndependentReviewImagePage{}, err
		}
		scanned := 0
		for rows.Next() {
			var item reviewimagefacade.IndependentReviewImageOption
			var raw json.RawMessage
			if err = rows.Scan(&item.Digest, &item.DisplayName, &raw); err != nil {
				rows.Close()
				return reviewimagefacade.IndependentReviewImagePage{}, err
			}
			scanned++
			scanCursor = item.Digest
			if !independentReviewCapabilitiesContain(raw, []string{"review.provider.gateway-client", "review.fixed-provider-identity"}) {
				continue
			}
			set := independentReviewCapabilitySet(raw)
			for capability := range set {
				item.Capabilities = append(item.Capabilities, capability)
			}
			sort.Strings(item.Capabilities)
			item.Available = true
			items = append(items, item)
			if len(items) > limit {
				break
			}
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return reviewimagefacade.IndependentReviewImagePage{}, rowErr
		}
		if scanned < 100 || len(items) > limit {
			break
		}
	}
	page := reviewimagefacade.IndependentReviewImagePage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].Digest
	}
	return page, nil
}

func (IndependentReviewImageCatalog) ValidateCustomIndependentReviewImage(ctx context.Context, tx pgx.Tx, digest string, required []string) error {
	// Lock every admissible exact-digest record that can satisfy this probe.
	// Task creation keeps the lock through its execution-snapshot commit, so a
	// concurrent inventory update cannot turn the accepted image MISSING,
	// DRIFTED, or capability-incompatible between validation and freeze.
	rows, err := tx.Query(ctx, `SELECT capabilities_json FROM workforce_worker_images
WHERE image_digest=$1 AND role='attempt' AND contract_version='seven.worker-image/v1'
  AND admission_status='ADMITTED' AND presence_status='PRESENT' FOR SHARE`, strings.ToLower(strings.TrimSpace(digest)))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var capabilities json.RawMessage
		if err = rows.Scan(&capabilities); err != nil {
			return err
		}
		if independentReviewCapabilitiesContain(capabilities, required) {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return reviewimagefacade.ErrIndependentReviewImageUnavailable
}

func (IndependentReviewImageCatalog) ValidateAnyTaskHarnessForIndependentReview(ctx context.Context, tx pgx.Tx, required []string) error {
	rows, err := tx.Query(ctx, `SELECT capabilities_json FROM workforce_harness_releases
WHERE status='ACTIVE' AND contract_version='seven.worker-image/v1'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var capabilities json.RawMessage
		if err = rows.Scan(&capabilities); err != nil {
			return err
		}
		if independentReviewCapabilitiesContain(capabilities, required) {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return reviewimagefacade.ErrIndependentReviewImageUnavailable
}

func (IndependentReviewImageCatalog) ValidateTaskHarnessForIndependentReview(ctx context.Context, tx pgx.Tx, digest string, required []string) error {
	var capabilities json.RawMessage
	err := tx.QueryRow(ctx, `SELECT capabilities_json FROM workforce_harness_releases
WHERE image_digest=$1 AND status='ACTIVE' AND contract_version='seven.worker-image/v1' FOR SHARE`, strings.ToLower(strings.TrimSpace(digest))).Scan(&capabilities)
	if err != nil {
		if err == pgx.ErrNoRows {
			return reviewimagefacade.ErrIndependentReviewImageUnavailable
		}
		return err
	}
	if !independentReviewCapabilitiesContain(capabilities, required) {
		return reviewimagefacade.ErrIndependentReviewImageUnavailable
	}
	return nil
}

func independentReviewCapabilitiesContain(raw json.RawMessage, required []string) bool {
	available := independentReviewCapabilitySet(raw)
	for _, want := range required {
		if _, ok := available[strings.TrimSpace(want)]; !ok {
			return false
		}
	}
	return true
}

func independentReviewCapabilitySet(raw json.RawMessage) map[string]struct{} {
	available := make(map[string]struct{})
	var direct []string
	if json.Unmarshal(raw, &direct) == nil {
		for _, item := range direct {
			available[strings.TrimSpace(item)] = struct{}{}
		}
		return available
	}

	// Worker image inventory persists the seven-worker-cli image probe as an
	// object whose capabilities are explicit name/status records. A capability
	// is usable only when the probe recorded it as supported; admitting an
	// arbitrary name with an unavailable status would weaken Profile Probe.
	var probe struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if json.Unmarshal(raw, &probe) != nil || len(probe.Capabilities) == 0 {
		return available
	}
	var records []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if json.Unmarshal(probe.Capabilities, &records) == nil {
		for _, record := range records {
			if strings.EqualFold(strings.TrimSpace(record.Status), "supported") {
				available[strings.TrimSpace(record.Name)] = struct{}{}
			}
		}
		return available
	}
	var nested []string
	if json.Unmarshal(probe.Capabilities, &nested) == nil {
		for _, item := range nested {
			available[strings.TrimSpace(item)] = struct{}{}
		}
	}
	return available
}
