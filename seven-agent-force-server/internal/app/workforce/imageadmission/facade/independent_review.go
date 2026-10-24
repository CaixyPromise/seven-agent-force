// Package facade exposes admitted-image validation for Independent Reviewer
// policy.  Review Policy receives only a narrow capability check, never a
// Worker inventory, Docker fact, lease, or image-admission store.
package facade

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrIndependentReviewImageUnavailable = errors.New("INDEPENDENT_REVIEW_IMAGE_UNAVAILABLE")

// IndependentReviewImageResolver verifies an exact, admitted Worker image or
// the admitted Task Harness identity in the caller's database transaction.
// The Image Admission module owns the query shapes and admission semantics.
type IndependentReviewImageResolver interface {
	ValidateCustomIndependentReviewImage(context.Context, pgx.Tx, string, []string) error
	ValidateAnyTaskHarnessForIndependentReview(context.Context, pgx.Tx, []string) error
	ValidateTaskHarnessForIndependentReview(context.Context, pgx.Tx, string, []string) error
	ListIndependentReviewImages(context.Context, pgx.Tx, string, string, int) (IndependentReviewImagePage, error)
}

type IndependentReviewImageOption struct {
	Digest       string   `json:"digest"`
	DisplayName  string   `json:"displayName"`
	Capabilities []string `json:"capabilities"`
	Available    bool     `json:"available"`
}

type IndependentReviewImagePage struct {
	Items      []IndependentReviewImageOption `json:"items"`
	NextCursor string                         `json:"nextCursor,omitempty"`
}
