package facade

import "context"

type FileAssetBindingFacade interface {
	BindUploadedFile(ctx context.Context, command BindUploadedFileCommand) (*FileReferenceDTO, error)
}

// ConfigAssetFacade owns the finite CONFIG_ASSET lifecycle. Keeping this
// separate from FileAssetBindingFacade prevents unrelated business modules
// from turning a generic fileId into an arbitrary configuration reference.
type ConfigAssetFacade interface {
	BindConfigAsset(ctx context.Context, command BindConfigAssetCommand) error
	UpdateConfigAssetPolicy(ctx context.Context, command UpdateConfigAssetPolicyCommand) error
	ClearConfigAsset(ctx context.Context, configID int64) error
	CaptureConfigAssetBinding(ctx context.Context, command CaptureConfigAssetBindingCommand) (ConfigAssetBindingState, error)
	RestoreConfigAssetBinding(ctx context.Context, command RestoreConfigAssetBindingCommand) error
	OpenConfigAsset(ctx context.Context, configID int64) (*ConfigAssetOpenResult, error)
}

// WorkforceInputAssetFacade is a narrow application-to-application bridge.
// A caller may claim only a file authorized by the current authenticated
// upload credential. Background work can later open the immutable bytes only
// with the returned opaque handle and the server-owned organization scope.
// Neither operation exposes storage paths, visit URLs, references or tokens.
type WorkforceInputAssetFacade interface {
	ClaimWorkforceInput(ctx context.Context, command ClaimWorkforceInputCommand) (*WorkforceInputClaim, error)
	OpenWorkforceInput(ctx context.Context, command OpenWorkforceInputCommand) (*WorkforceInputOpenResult, error)
	ReleaseWorkforceInput(ctx context.Context, command ReleaseWorkforceInputCommand) error
}

type Facades struct {
	Assets          FileAssetBindingFacade
	ConfigAssets    ConfigAssetFacade
	WorkforceInputs WorkforceInputAssetFacade
}
