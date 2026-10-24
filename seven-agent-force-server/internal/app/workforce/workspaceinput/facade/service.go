package facade

import (
	"context"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

type TaskInputSelection struct {
	Mode                      string
	WorkspaceSourceRevisionID int64
	ReferenceAssetIDs         []int64
	GitPlan                   domain.GitPlan
}
type SourceBundle struct {
	RevisionID                             int64
	Kind                                   string
	ProjectID                              int64
	ProjectVersionID                       int64
	RepositoryConfigDigest                 string
	RefType                                string
	RefName                                string
	Commit, Tree, IdentityDigest           string
	ObjectKey, ContentSHA256, ObjectSHA256 string
	SizeBytes                              int64
}
type Reference struct {
	ID            int64
	DisplayName   string
	ContentType   string
	ContentSHA256 string
	ObjectKey     string
	ObjectSHA256  string
	SizeBytes     int64
}
type FrozenTaskInput struct {
	Snapshot   domain.TaskInputSnapshot
	Source     SourceBundle
	References []Reference
}
type Resolver interface {
	ResolveExecutionInput(context.Context, int64, TaskInputSelection) (FrozenTaskInput, error)
}
type Adapter struct{ Service *application.Service }

func (a Adapter) ResolveExecutionInput(ctx context.Context, tenant int64, selection TaskInputSelection) (FrozenTaskInput, error) {
	value, err := a.Service.FreezeTaskInput(ctx, tenant, selection.Mode, selection.WorkspaceSourceRevisionID, selection.ReferenceAssetIDs, selection.GitPlan)
	if err != nil {
		return FrozenTaskInput{}, err
	}
	result := FrozenTaskInput{Snapshot: value.Snapshot, Source: SourceBundle{RevisionID: value.Source.ID, Kind: value.Source.Kind, ProjectID: value.Source.ProjectID, ProjectVersionID: value.Source.ProjectVersionID, RepositoryConfigDigest: value.Source.RepositoryConfigDigest, RefType: value.Source.RefType, RefName: value.Source.RefName, Commit: value.Source.ResolvedCommit, Tree: value.Source.ResolvedTree, IdentityDigest: value.Source.SourceIdentityDigest, ObjectKey: value.Source.RepositoryBundleObjectKey, ContentSHA256: value.Source.RepositoryBundleContentSHA256, ObjectSHA256: value.Source.RepositoryBundleObjectSHA256, SizeBytes: value.Source.RepositoryBundleSizeBytes}, References: make([]Reference, 0, len(value.References))}
	for _, item := range value.References {
		result.References = append(result.References, Reference{ID: item.ID, DisplayName: item.DisplayName, ContentType: item.ContentType, ContentSHA256: item.ContentSHA256, ObjectKey: item.ObjectKey, ObjectSHA256: item.ObjectSHA256, SizeBytes: item.SizeBytes})
	}
	return result, nil
}
