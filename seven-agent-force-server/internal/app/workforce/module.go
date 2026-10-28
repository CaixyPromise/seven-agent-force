package workforce

import (
	workitemapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/application"
	workitemcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/controller"
	workiteminfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/infrastructure"

	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	completionapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	completioncontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/controller"
	completioninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/infrastructure"
	executionaccessapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/application"
	executionaccesscontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/controller"
	executionaccessdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	executionaccessfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	executionaccessinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/infrastructure"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	routinginfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/infrastructure"
	interactionapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/application"
	interactioncontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/controller"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	interactioninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/infrastructure"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	postprocessinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/infrastructure"
	continuationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	continuationcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/controller"
	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverycontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/controller"
	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	deliveryinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/infrastructure"
	hookapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/application"
	hookcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/controller"
	hookinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/imageadmission"
	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	workforcedb "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/database"
	machineidem "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/machineidempotency"
	openapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/application"
	opencontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/controller"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	projectcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/controller"
	projectdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/infrastructure"
	providerapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/application"
	providercontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/controller"
	providerinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/infrastructure"
	invocationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/application"
	invocationcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/controller"
	invocationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/infrastructure"
	processorapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/application"
	processorcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/controller"
	processorinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
	reviewrunapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/application"
	reviewruncontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/controller"
	reviewruninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/infrastructure"
	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	taskapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/application"
	taskcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/controller"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	analyticsapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskanalytics/application"
	analyticscontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskanalytics/controller"
	analyticsinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskanalytics/infrastructure"
	dispatchapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	dispatchcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/controller"
	dispatchinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/infrastructure"
	tasknotificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/application"
	tasknotificationcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/controller"
	tasknotificationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/infrastructure"
	registryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/application"
	registrycontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/controller"
	registrydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
	registryinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/infrastructure"
	workspaceinput "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput"
	workspaceinputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	rabbitmqinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/rabbitmq"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	taskExchange = "seven.workforce.tasks"
	taskDLX      = "seven.workforce.tasks.dlx"
)

type Module struct {
	workitemHandler         *workitemcontroller.Handler
	taskHandler             *taskcontroller.Handler
	analyticsHandler        *analyticscontroller.Handler
	taskNotificationHandler *tasknotificationcontroller.Handler
	providerHandler         *providercontroller.Handler
	projectHandler          *projectcontroller.Handler
	hookHandler             *hookcontroller.Handler
	workerHandler           *registrycontroller.Handler
	machineHandler          *dispatchcontroller.MachineHandler
	invocationHandler       *invocationcontroller.Handler
	processorHandler        *processorcontroller.Handler
	reviewRunHandler        *reviewruncontroller.Handler
	openIntegrationHandler  *opencontroller.Handler
	processorService        *processorapp.Service
	continuationHandler     *continuationcontroller.Handler
	completionHandler       *completioncontroller.Handler
	interactionHandler      *interactioncontroller.Handler
	executionAccessHandler  *executionaccesscontroller.Handler
	deliveryHandler         *deliverycontroller.Handler
	deliveryService         *deliveryapp.Service
	deliveryMachine         *deliveryapp.MachineService
	reviewCoordinator       *deliveryapp.ReviewCoordinator
	workspaceInputModule    *workspaceinput.Module
	db                      *pgxpool.Pool
	localMode               bool
	oplog                   adminfacade.OperationLogger
	cancel                  context.CancelFunc
}

type Dependencies struct {
	Notifications notificationfacade.NotificationClient
	Authorization authorizationfacade.AuthFacade
}

type projectSourceCredentialResolver struct {
	cipher projectinfra.RepositoryCredentialCipher
}

func (r projectSourceCredentialResolver) ResolveSourceCredential(ctx context.Context, value workspaceinputapp.EncryptedRepositoryCredential) (workspaceinputapp.SourceCredential, error) {
	if value.Version == 0 && value.Ciphertext == "" && value.EDEK == "" && value.WrapKeyRef == "" {
		return workspaceinputapp.SourceCredential{}, nil
	}
	credential, err := r.cipher.DecryptRepositoryCredential(ctx, value.Ciphertext, value.EDEK, value.WrapKeyRef)
	if err != nil {
		return workspaceinputapp.SourceCredential{}, err
	}
	return workspaceinputapp.SourceCredential{Username: credential.Username, Password: credential.Token}, nil
}

func Install(deps bootstrapruntime.ModuleDeps, integrations Dependencies) (*Module, error) {
	if !deps.Config.Workforce.Enabled {
		return nil, nil
	}
	if deps.Infra.Datasource == nil || deps.Infra.Datasource.Dialect() != "postgres" {
		return nil, fmt.Errorf("workforce module requires the Framework PostgreSQL datasource")
	}
	if deps.Infra.RabbitMQ == nil || !deps.Infra.RabbitMQ.Enabled() {
		return nil, fmt.Errorf("workforce module requires the Framework RabbitMQ client")
	}
	if err := deps.Infra.RabbitMQ.DeclareTopology(rabbitmqinfra.Topology{
		Exchanges: []rabbitmqinfra.ExchangeOptions{{Name: taskExchange, Type: "direct", Durable: true}, {Name: taskDLX, Type: "direct", Durable: true}},
	}); err != nil {
		return nil, fmt.Errorf("declare workforce task topology: %w", err)
	}

	runtimeCfg := RuntimeConfig(deps.Config, deps.Config.Datasource.Bootstrap.MigrationsDir)
	db, err := workforcedb.Open(context.Background(), runtimeCfg.Database)
	if err != nil {
		return nil, err
	}
	artifacts, err := artifactstore.Open(context.Background(), runtimeCfg.Artifact)
	if err != nil {
		db.Close()
		return nil, err
	}
	taskNotificationStore := tasknotificationinfra.NewStore(db, deps.IDGen)
	// The Result Processor authoring store owns immutable releases, but the
	// Python/bootstrap/module identities are resolved from the exact admitted
	// Harness image at Task snapshot time.  A Server build string is never a
	// substitute for an image identity.
	processorStore := processorinfra.New(db, deps.IDGen)
	processorStore.SetTaskNotificationRecorder(taskNotificationStore)
	processorService := processorapp.NewService(processorStore, artifacts)
	// Built-in templates and the official default processor are normal
	// immutable assets. Startup does not bind a tenant default automatically.
	if err := processorService.BootstrapBuiltins(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("bootstrap Result Processor assets: %w", err)
	}
	processorHandler := processorcontroller.New(processorService)
	store := taskinfra.NewStore(db, deps.IDGen)
	analyticsHandler := analyticscontroller.New(analyticsapp.New(analyticsinfra.New(db)))
	store.SetNotificationSubscriptions(taskNotificationStore)
	store.SetTaskNotificationRecorder(taskNotificationStore)
	providerStore := providerinfra.NewStore(db)
	modelDiscovery, err := providerinfra.NewModelDiscovery(runtimeCfg.ProviderSSRF)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("configure workforce model discovery: %w", err)
	}
	providerService := providerapp.NewService(providerStore, deps.Security.SecretValue, modelDiscovery, deps.IDGen)
	providerHandler := providercontroller.New(providerService)
	providerContract := providerGatewayContract(runtimeCfg.ProviderSSRF)
	facadeContract := sha256.Sum256([]byte("seven.local-provider-facade\x00127.0.0.1:8787\x00raw-json-sse"))
	var hookHandler *hookcontroller.Handler
	if runtimeCfg.ManagedHooks.Enabled {
		hookStore := hookinfra.NewStore(db, deps.IDGen)
		hookHandler = hookcontroller.New(hookapp.NewService(hookStore, artifacts))
	}
	service := taskapp.NewService(store, processorService.Resolver())
	service.BindModelSelectionValidator(providerService)
	if validator, ok := integrations.Notifications.(taskapp.NotificationSelectionValidator); ok {
		service.BindNotificationSelectionValidator(validator)
	}
	registryStore := registryinfra.New(db, deps.IDGen)
	registryService := registryapp.New(registryStore, workerQueueTopology{client: deps.Infra.RabbitMQ}, deps.IDGen)
	registryService.SetMetricFailureHandler(func(err error) {
		deps.Logger.Warn("record Worker metric sample", zap.Error(err))
	})
	workerHandler := registrycontroller.New(registryService)
	invocationAdapter, err := invocationinfra.NewResponsesAdapter(runtimeCfg.ProviderSSRF)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("configure Provider Invocation Adapter: %w", err)
	}
	invocationService := invocationapp.NewService(invocationinfra.NewStore(db), providerService, invocationAdapter, deps.IDGen)
	providerService.SetInvocationCanceller(invocationService)
	service.SetInvocationCanceller(invocationService)
	invocationHandler := invocationcontroller.New(invocationService, registryService)
	dispatchStore := dispatchinfra.New(db, deps.IDGen)
	assignmentResolver := routinginfra.NewResolver(routingfacade.PlatformIdentity{
		ProviderProxyImageDigest: runtimeCfg.ProviderProxy.ImageDigest,
		ProviderProxyVersion:     runtimeCfg.ProviderProxy.Version,
		LocalFacadeVersion:       buildinfo.Version,
		LocalFacadeDigest:        hex.EncodeToString(facadeContract[:]),
		GatewayIdentity:          buildinfo.String(),
		GatewayContractDigest:    hex.EncodeToString(providerContract[:]),
		ProviderAdapterIdentity:  "openai-responses-compatible/raw-pass-through",
	})
	dispatchStore.SetAssignmentResolver(assignmentResolver)
	dispatchStore.SetTaskNotificationRecorder(taskNotificationStore)
	dispatchService := dispatchapp.New(dispatchStore)
	postprocessStore := postprocessinfra.New(db, deps.IDGen)
	postprocessStore.SetTaskNotificationRecorder(taskNotificationStore)
	// Slice 4 starts the durable ReviewRun scheduler. Assignment remains safe
	// when no review-capable Worker is registered: the Store leaves the Run
	// pending with an explicit capacity reason instead of widening authority.
	reviewRunStore := reviewruninfra.New(db, deps.IDGen)
	reviewRunStore.SetAssignmentResolver(assignmentResolver)
	reviewRunStore.SetTaskNotificationRecorder(taskNotificationStore)
	reviewRunStore.SetProviderInvocationCanceller(invocationService)
	reviewRunService := reviewrunapp.NewService(reviewRunStore)
	reviewRunHandler := reviewruncontroller.New(reviewRunService.HumanClosure(), reviewRunService.TaskReviewReader())
	reviewRunHandler.BindAuthorization(integrations.Authorization)
	reviewRunService.BindExecutionInputs(service.IndependentReviewInputReader())
	postprocessStore.SetReviewRunGate(reviewRunService.FinalizationGate())
	store.SetReviewRunLifecycle(reviewRunService.TaskLifecycle())
	store.SetReviewRunWorkspaceRetention(reviewRunService.WorkspaceRetention())
	service.BindReviewRunSummaryProvider(reviewRunService.TaskSummaryProvider())
	service.BindReviewRunReportProvider(reviewRunService.TaskReportProvider())
	continuationStore := continuationinfra.New(db, deps.IDGen)
	reviewRunStore.SetReviewRevisionTurnCreator(continuationStore)
	continuationService := continuationapp.New(continuationStore)
	continuationService.SetKeyWrapper(continuationKeyWrapper{service: deps.Security.SecretValue})
	continuationHandler := continuationcontroller.New(continuationService)
	completionStore := completioninfra.New(db, deps.IDGen, continuationStore, principalinfra.NewResolver())
	processorStore.SetCompletionCheckEvidenceReader(completionStore)
	completionHandler := completioncontroller.New(completionapp.New(completionStore))
	revisionAuthority := dispatchinfra.NewRevisionSafePointAuthority()
	completionRevisions := completionapp.NewRevisions(db, deps.IDGen, completioninfra.NewRevisionStore(), principalinfra.NewResolver(), revisionAuthority, continuationStore)
	revisionAuthority.WithRevisionLookup(completionRevisions)
	completionHandler.WithRevisions(completionRevisions)
	interactionAuthority := dispatchinfra.NewInteractionAuthority(deps.IDGen, principalinfra.NewResolver(), registryStore)
	interactionService := interactionapp.New(db, deps.IDGen, interactioninfra.New(), interactionAuthority, interactionAuthority).
		WithContinuation(continuationStore, continuationapp.NewInteractionResume(continuationStore)).
		WithSummaryAuthority(interactionAuthority).
		WithEventRecorder(interactionEvents{notifications: taskNotificationStore})
	continuationStore.WithInteractionRestoreQuestions(interactionService)
	interactionHandler := interactioncontroller.New(interactionService)
	executionAccessAuthority := dispatchinfra.NewExecutionAccessAuthority(deps.IDGen, principalinfra.NewResolver(), registryStore)
	executionAccessService := executionaccessapp.New(db, deps.IDGen, executionaccessinfra.New(), executionAccessAuthority, executionAccessAuthority, executionAccessCipher{service: deps.Security.SecretValue}, executionAccessProjector{target: interactionService}).
		WithContinuation(continuationStore, continuationapp.NewOperationResume(continuationStore))
	executionAccessHandler := executionaccesscontroller.New(executionAccessService)
	store.SetInteractionLifecycle(interactionService)
	store.SetExecutionAccessLifecycle(executionAccessService)
	interactionSuspend := continuationapp.NewInteractionSuspend(continuationStore, continuationKeyWrapper{service: deps.Security.SecretValue}, continuationapp.InteractionObjectReaderFunc(func(ctx context.Context, key string) (io.ReadCloser, error) { return artifacts.Get(ctx, key) }), interactionService)
	interactionSuspend.WithOperationSuspendGate(executionAccessService)
	deliveryStore := deliveryinfra.New(db, deps.IDGen)
	deliveryStore.SetFrozenRepositoryPlanReader(store)
	deliveryStore.SetTaskNotificationRecorder(taskNotificationStore)
	deliveryStore.SetToolAuthority(dispatchinfra.NewDeliveryToolAuthority(interactionAuthority))
	deliveryStore.SetConversationProjector(deliveryConversationProjector{target: interactionService})
	store.SetDeliveryLifecycle(deliveryStore)
	deliveryService := deliveryapp.New(deliveryStore, string(deps.Config.Platform.Mode) == "local")
	deliveryHandler := deliverycontroller.New(deliveryService)
	deliveryMachine := deliveryapp.NewMachineService(deliveryStore, deliveryCheckpointKeyResolver{service: deps.Security.SecretValue})
	reviewCoordinator := deliveryapp.NewReviewCoordinator(deliveryStore, deliveryinfra.NewReviewHTTPAdapter())
	deliveryService.SetReviewReconciler(reviewCoordinator)
	projectStore := projectinfra.New(db, deps.IDGen)
	workspaceInputModule := workspaceinput.New(db, projectStore, artifacts, deps.IDGen)
	workspaceInputModule.BindCredentials(projectSourceCredentialResolver{cipher: projectinfra.RepositoryCredentialCipher{Secrets: deps.Security.SecretValue}})
	service.SetTaskInputResolver(workspaceInputModule.Facade)
	projectService := projectapp.New(projectStore)
	deliveryStore.SetDeliveryRepositoryBindingReader(projectService)
	projectService.BindReferenceValidator(projectStore)
	projectService.BindRepositoryVerifier(projectinfra.RepositoryVerifier{AllowLocalLab: string(deps.Config.Platform.Mode) == "local"})
	projectService.BindCredentialCipher(projectinfra.RepositoryCredentialCipher{Secrets: deps.Security.SecretValue})
	projectService.BindSourcePreparer(projectapp.SourcePreparerFunc(func(ctx context.Context, tenantID, actorID, projectID, versionID int64) error {
		project, err := projectStore.Get(ctx, tenantID, projectID)
		if err != nil || project.VersionID != versionID {
			return projectdomain.ErrConflict
		}
		err = workspaceInputModule.Service.PrepareProjectSource(ctx, tenantID, actorID, workspaceinputapp.GitSourceRequest{
			ProjectID: projectID, ProjectVersionID: versionID, RefType: "BRANCH", RefName: project.DefaultRefName,
			IdempotencyKey: "project-source:" + strconv.FormatInt(projectID, 10) + ":" + strconv.FormatInt(versionID, 10),
		})
		return err
	}))
	service.BindProjectCompiler(projectService)
	runtimeCatalog := routinginfra.NewRuntimeCatalog(db, projectService)
	service.BindRuntimeCatalog(runtimeCatalog)
	projectHandler := projectcontroller.New(projectService)
	var openIntegrationHandler *opencontroller.Handler
	var openProjector *openapp.Projector
	var openWebhookWorker *openapp.WebhookWorker
	if deps.Config.Workforce.OpenIntegration.Enabled {
		openStore := openinfra.NewStore(db, deps.IDGen, runtimeCfg.Harness.Alias)
		store.SetSubmissionGuard(openStore)
		service.SetScenarioReadinessChecker(openStore)
		urlValidator := openinfra.NewWebhookURLValidator(outboundurl.NewOutboundURLGuard(outboundurl.Options{}), deps.Config.Workforce.OpenIntegration.AllowLocalWebhookEndpoint && string(deps.Config.Platform.Mode) == "local")
		openService := openapp.NewService(openStore, service, processorService, deps.Security.SecretValue, urlValidator, runtimeCfg.Harness.Alias)
		openService.SetSelectionCatalogs(providerService, projectService, runtimeCatalog)
		openIntegrationHandler = opencontroller.New(openService)
		openCfg := deps.Config.Workforce.OpenIntegration
		openProjector = openapp.NewProjector(openStore, openCfg.ProjectorInterval, deps.Logger)
		openWebhookWorker = openapp.NewWebhookWorker(openStore, deps.Security.SecretValue, urlValidator, deps.Logger, openCfg.DeliveryBatchSize, time.Duration(openCfg.DeliveryLeaseSeconds)*time.Second, time.Duration(openCfg.HTTPTimeoutSeconds)*time.Second, openCfg.ProjectorInterval)
	}
	taskHandler := taskcontroller.New(service, store, artifacts, postprocessStore)
	taskHandler.SetResultProcessorService(processorService)
	taskHandler.SetInteractionSummaries(interactionService)
	taskNotificationService := tasknotificationapp.NewService(taskNotificationStore, integrations.Notifications != nil)
	if readiness, ok := integrations.Notifications.(tasknotificationapp.PlatformReadinessResolver); ok {
		taskNotificationService.BindPlatformReadiness(readiness)
	}
	taskNotificationHandler := tasknotificationcontroller.New(taskNotificationService)
	workitemStore := workiteminfra.New(db, deps.IDGen, principalinfra.NewResolver())
	continuationService.SetWorkItemMembership(workitemStore)
	workitemService := workitemapp.New(workitemStore, service, processorService, deps.IDGen, invocationinfra.NewStore(db))
	workitemHandler := workitemcontroller.New(workitemService)
	idempotency := machineidem.New(db, registryService, deps.Security.SecretValue)
	workerHandler.SetMachineIdempotency(idempotency)
	machineHandler := dispatchcontroller.NewMachine(dispatchService, dispatchStore, store, registryService, postprocessStore, processorService, continuationService, deliveryMachine, idempotency, deps.IDGen)
	machineHandler.SetCompletionChecks(completionStore)
	machineHandler.SetCompletionRevisions(completionRevisions)
	machineHandler.SetInteractions(interactionService)
	machineHandler.SetExecutionAccess(executionAccessService)
	machineHandler.SetDeliveryTools(deliveryService)
	machineHandler.SetInteractionSuspend(interactionSuspend)
	machineHandler.SetWorkspaceInput(workspaceInputModule.Service)
	machineHandler.SetWorkspaceRetention(service)
	machineHandler.SetReviewRunControl(reviewRunService.MachineControl())
	moduleCtx, cancel := context.WithCancel(context.Background())
	if openProjector != nil {
		go openProjector.Run(moduleCtx)
	}
	if openWebhookWorker != nil {
		go openWebhookWorker.Run(moduleCtx)
	}
	go taskapp.NewOutboxPublisher(store, frameworkTaskPublisher{client: deps.Infra.RabbitMQ}, deps.Logger).Run(moduleCtx)
	if integrations.Notifications != nil {
		taskNotificationPublisher := tasknotificationinfra.NewPublisher(taskNotificationStore, integrations.Notifications)
		taskNotificationPublisher.SetLogger(deps.Logger)
		go taskNotificationPublisher.Run(moduleCtx)
	}
	go dispatchService.Run(moduleCtx, deps.Logger)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-moduleCtx.Done():
				return
			case <-ticker.C:
				if err := workitemService.Reconcile(moduleCtx); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile development work item", zap.Error(err))
				}
				if err := projectService.ReconcileSourcePreparations(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile Project source preparation", zap.Error(err))
				}
				if _, err := executionAccessService.ReconcileExpired(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile controlled network operation", zap.Error(err))
				}
				if _, err := reviewRunService.AssignAvailable(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("assign independent ReviewRun", zap.Error(err))
				}
				if _, err := continuationStore.ReconcileImmediateMaterializations(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile immediate Result archive", zap.Error(err))
				}
				if _, err := deliveryStore.ReconcileResultActions(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile Workforce Result Delivery actions", zap.Error(err))
				}
				if _, err := postprocessStore.ReconcileTimeouts(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile Workforce post-process timeout", zap.Error(err))
				}
				if _, err := reviewRunService.ReconcileExpired(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile independent ReviewRun lease", zap.Error(err))
				}
				if _, err := deliveryStore.ReconcileRetries(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile Workforce Delivery retry", zap.Error(err))
				}
				if _, err := deliveryStore.ReconcileUnknown(moduleCtx, 32); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile Workforce Delivery unknown outcome", zap.Error(err))
				}
				if _, err := reviewCoordinator.Reconcile(moduleCtx, 16); err != nil && moduleCtx.Err() == nil {
					deps.Logger.Warn("reconcile Workforce draft review", zap.Error(err))
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-moduleCtx.Done():
				return
			case <-ticker.C:
				_ = idempotency.Cleanup(moduleCtx)
			}
		}
	}()
	go func() {
		cleanup := func() {
			if deleted, err := registryService.DeleteExpiredMetrics(moduleCtx, time.Now()); err != nil && moduleCtx.Err() == nil {
				deps.Logger.Warn("delete expired Worker metric samples", zap.Error(err))
			} else if deleted > 0 {
				deps.Logger.Info("deleted expired Worker metric samples", zap.Int64("rows", deleted))
			}
		}
		cleanup()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-moduleCtx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()
	go imageadmission.RunWorkerEvidenceAdopter(moduleCtx, db, runtimeCfg.Harness, deps.IDGen)
	return &Module{workitemHandler: workitemHandler, taskHandler: taskHandler, analyticsHandler: analyticsHandler, projectHandler: projectHandler, completionHandler: completionHandler, interactionHandler: interactionHandler, executionAccessHandler: executionAccessHandler, taskNotificationHandler: taskNotificationHandler, openIntegrationHandler: openIntegrationHandler, providerHandler: providerHandler, hookHandler: hookHandler, workerHandler: workerHandler, machineHandler: machineHandler, invocationHandler: invocationHandler, processorHandler: processorHandler, reviewRunHandler: reviewRunHandler, processorService: processorService, continuationHandler: continuationHandler, deliveryHandler: deliveryHandler, deliveryService: deliveryService, deliveryMachine: deliveryMachine, reviewCoordinator: reviewCoordinator, workspaceInputModule: workspaceInputModule, db: db, localMode: string(deps.Config.Platform.Mode) == "local", cancel: cancel}, nil
}

type continuationKeyWrapper struct{ service secretvalueinfra.Service }
type deliveryCheckpointKeyResolver struct{ service secretvalueinfra.Service }
type executionAccessCipher struct{ service secretvalueinfra.Service }
type executionAccessProjector struct {
	target interactionfacade.ExternalProjector
}

func (c executionAccessCipher) Encrypt(ctx context.Context, plain []byte) (executionaccessdomain.EncryptedDescriptor, error) {
	value, err := c.service.EncryptBytes(ctx, plain)
	return executionaccessdomain.EncryptedDescriptor{Ciphertext: value.CiphertextB64, EDEK: value.EDEKB64, KeyRef: value.WrapKeyRef}, err
}

func (c executionAccessCipher) Decrypt(ctx context.Context, value executionaccessdomain.EncryptedDescriptor) ([]byte, error) {
	return c.service.DecryptBytes(ctx, secretvalueinfra.SecretValue{CiphertextB64: value.Ciphertext, EDEKB64: value.EDEK, WrapKeyRef: value.KeyRef})
}

func (p executionAccessProjector) ProjectExecutionOperation(ctx context.Context, tx pgx.Tx, value executionaccessfacade.Projection) error {
	if p.target == nil {
		return executionaccessdomain.ErrUnavailable
	}
	projection := map[string]any{
		"operationId":          strconv.FormatInt(value.OperationID, 10),
		"version":              strconv.FormatInt(value.Version, 10),
		"status":               value.Status,
		"purpose":              value.Purpose,
		"targets":              value.Targets,
		"resultDeliveryStatus": value.ResultDeliveryStatus,
		"agentContinued":       value.AgentContinued,
	}
	if value.ExitCode != nil || value.ResultSummary != "" {
		projection["result"] = map[string]any{"exitCode": value.ExitCode, "summary": value.ResultSummary}
	}
	body, err := json.Marshal(projection)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	return p.target.ProjectExternalTx(ctx, tx, interactionfacade.ExternalProjection{ID: value.OperationID, TenantID: value.TenantID, TaskID: value.TaskID, TaskTurnID: value.TaskTurnID, AttemptID: value.AttemptID, Version: value.Version, Type: "OPERATION", SourceKind: "EXECUTION_OPERATION", SourceKey: strconv.FormatInt(value.OperationID, 10), Digest: hex.EncodeToString(digest[:]), Body: body, AttentionRequired: value.AttentionRequired, OccurredAt: value.OccurredAt})
}

type deliveryConversationProjector struct {
	target interactionfacade.ExternalProjector
}

func (p deliveryConversationProjector) ProjectDeliveryTx(ctx context.Context, tx pgx.Tx, value deliveryfacade.Projection) error {
	if p.target == nil {
		return errors.New("DELIVERY_PROJECTION_UNAVAILABLE")
	}
	body, err := json.Marshal(map[string]any{
		"intentId": strconv.FormatInt(value.IntentID, 10), "requestId": decimalProjectionID(value.RequestID),
		"version": strconv.FormatInt(value.Version, 10), "intentVersion": strconv.FormatInt(value.IntentVersion, 10), "requestVersion": decimalProjectionID(value.RequestVersion), "status": value.Status, "summary": value.Summary,
		"deliveryMode": value.DeliveryMode, "authorization": value.Authorization,
		"repositoryName": value.RepositoryName, "destinationBranch": value.DestinationBranch,
		"resultCommit": value.ResultCommit, "deliveryCommit": value.DeliveryCommit,
		"pullRequestUrl": value.PullRequestURL, "remoteReadbackVerified": value.RemoteReadbackVerified,
		"reasonCode": value.ReasonCode, "allowedActions": value.AllowedActions,
	})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	// Delivery records its own approval notifications with a committed event
	// occurrence. Reprojecting this card must not recreate an Agent question
	// notification under the immutable intent ID.
	return p.target.ProjectExternalTx(ctx, tx, interactionfacade.ExternalProjection{ID: value.ID, TenantID: value.TenantID, TaskID: value.TaskID, TaskTurnID: value.TaskTurnID, AttemptID: value.AttemptID, Version: value.Version, Type: "DELIVERY", SourceKind: "GIT_DELIVERY", SourceKey: strconv.FormatInt(value.TaskTurnID, 10), Digest: hex.EncodeToString(digest[:]), Body: body, OccurredAt: value.OccurredAt})
}

func decimalProjectionID(value int64) any {
	if value <= 0 {
		return nil
	}
	return strconv.FormatInt(value, 10)
}

func (r deliveryCheckpointKeyResolver) UnwrapCheckpointKey(ctx context.Context, wrapped string) ([]byte, error) {
	var envelope continuationapp.KeyEnvelope
	if err := json.Unmarshal([]byte(wrapped), &envelope); err != nil {
		return nil, err
	}
	return r.service.DecryptBytes(ctx, secretvalueinfra.SecretValue{CiphertextB64: envelope.Ciphertext, EDEKB64: envelope.EDEK, WrapKeyRef: envelope.KeyID})
}

func (w continuationKeyWrapper) Wrap(ctx context.Context, key []byte) (continuationapp.KeyEnvelope, error) {
	value, err := w.service.EncryptBytes(ctx, key)
	return continuationapp.KeyEnvelope{Ciphertext: value.CiphertextB64, EDEK: value.EDEKB64, KeyID: value.WrapKeyRef}, err
}

func (w continuationKeyWrapper) Unwrap(ctx context.Context, value continuationapp.KeyEnvelope) ([]byte, error) {
	return w.service.DecryptBytes(ctx, secretvalueinfra.SecretValue{CiphertextB64: value.Ciphertext, EDEKB64: value.EDEK, WrapKeyRef: value.KeyID})
}

// BindConfigFacade is deliberately late-bound because Framework System Config
// is installed before Workforce. The bridge remains a small facade dependency:
// Result Processor code never reaches config repositories or master keys.
func (m *Module) BindConfigFacade(config configfacade.ConfigFacade) error {
	if m == nil || config == nil {
		return nil
	}
	if m.processorService != nil {
		m.processorService.SetEnvironmentValueResolver(processorinfra.NewFrameworkConfigResolver(config))
	}
	if m.processorService != nil {
		return m.processorService.RegisterPersistedSecretReferences(context.Background())
	}
	return nil
}

func (m *Module) BindFileFacade(files filefacade.WorkforceInputAssetFacade) {
	if m != nil && m.workspaceInputModule != nil {
		m.workspaceInputModule.BindFiles(files)
	}
}

func (m *Module) BindOperationLogger(logger adminfacade.OperationLogger) {
	if m != nil {
		m.oplog = logger
	}
}

func providerGatewayContract(config workforceconfig.ProviderSSRFConfig) [sha256.Size]byte {
	type allowEntry struct {
		Hostname    string   `json:"hostname"`
		CIDRs       []string `json:"cidrs"`
		Ports       []int    `json:"ports"`
		AllowFakeIP bool     `json:"allowFakeIP"`
	}
	entries := make([]allowEntry, 0, len(config.AllowedList))
	for _, item := range config.AllowedList {
		cidrs := append([]string(nil), item.CIDRs...)
		ports := append([]int(nil), item.Ports...)
		sort.Strings(cidrs)
		sort.Ints(ports)
		entries = append(entries, allowEntry{Hostname: strings.ToLower(strings.TrimSpace(item.Hostname)), CIDRs: cidrs, Ports: ports, AllowFakeIP: item.AllowFakeIP})
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Hostname < entries[right].Hostname })
	canonical, _ := json.Marshal(struct {
		Contract                string       `json:"contract"`
		Operations              []string     `json:"operations"`
		Wire                    string       `json:"wire"`
		Enabled                 bool         `json:"ssrfEnabled"`
		AllowedList             []allowEntry `json:"allowedList"`
		LocalDevelopment        bool         `json:"localDevelopment"`
		MaxRequestBytes         int64        `json:"maxRequestBytes"`
		MaxResponseHeaderBytes  int64        `json:"maxResponseHeaderBytes"`
		RequestTimeoutNanos     int64        `json:"requestTimeoutNanos"`
		ResponseHeaderTimeoutNS int64        `json:"responseHeaderTimeoutNanos"`
		TLSHandshakeTimeoutNS   int64        `json:"tlsHandshakeTimeoutNanos"`
	}{
		Contract: "seven.provider-invocation", Operations: []string{"responses.create", "responses.compact"}, Wire: "raw-json-sse",
		Enabled: config.Enabled, AllowedList: entries, LocalDevelopment: config.LocalDevelopment,
		MaxRequestBytes: config.MaxRequestBytes, MaxResponseHeaderBytes: config.MaxResponseHeaderBytes,
		RequestTimeoutNanos: int64(config.RequestTimeout), ResponseHeaderTimeoutNS: int64(config.ResponseHeaderTimeout), TLSHandshakeTimeoutNS: int64(config.TLSHandshakeTimeout),
	})
	return sha256.Sum256(canonical)
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "workforce", Prefix: "/workforce"}
}

func (m *Module) Mount(router route.IRouter) {
	if m.workitemHandler != nil {
		m.workitemHandler.Mount(router, m.wrapPermission)
	}
	if m.completionHandler != nil {
		m.completionHandler.Mount(router, m.wrapPermission)
	}
	if m.interactionHandler != nil {
		m.interactionHandler.Mount(router, m.wrapPermission)
	}
	if m.executionAccessHandler != nil {
		m.executionAccessHandler.Mount(router, m.wrapPermission)
	}
	if m == nil || m.taskHandler == nil || router == nil {
		return
	}
	if m.analyticsHandler != nil {
		m.analyticsHandler.Mount(router, m.wrapPermission)
	}
	m.taskHandler.Mount(router, m.wrapPermission)
	if m.projectHandler != nil {
		m.projectHandler.Mount(router, m.wrapPermission)
	}
	if m.taskNotificationHandler != nil {
		m.taskNotificationHandler.Mount(router, m.wrapPermission, m.wrapTaskNotificationUpdate)
	}
	if m.openIntegrationHandler != nil {
		m.openIntegrationHandler.Mount(router, m.wrapPermission)
	}
	if m.continuationHandler != nil {
		m.continuationHandler.Mount(router, m.wrapPermission)
	}
	if m.deliveryHandler != nil {
		m.deliveryHandler.Mount(router, m.wrapPermission)
	}
	if m.workspaceInputModule != nil && m.workspaceInputModule.Handler != nil {
		m.workspaceInputModule.Handler.Mount(router, m.wrapPermission)
	}
	if m.providerHandler != nil {
		m.providerHandler.Mount(router, m.wrapPermission)
	}
	if m.hookHandler != nil {
		m.hookHandler.Mount(router, m.wrapPermission)
	}
	if m.workerHandler != nil {
		m.workerHandler.MountOperator(router, m.wrapPermission)
		m.workerHandler.MountMachine(router)
	}
	if m.machineHandler != nil {
		m.machineHandler.Mount(router)
	}
	if m.invocationHandler != nil {
		m.invocationHandler.Mount(router)
		m.invocationHandler.MountOperator(router, m.wrapPermission)
	}
	if m.processorHandler != nil {
		m.processorHandler.Mount(router, m.wrapPermission)
	}
	if m.reviewRunHandler != nil {
		m.reviewRunHandler.Mount(router, m.wrapPermission)
	}
}

func (m *Module) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.db != nil {
		m.db.Close()
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

func (m *Module) wrapPermission(permission string, handler app.HandlerFunc) app.HandlerFunc {
	if m.oplog != nil {
		switch permission {
		case "workforce:task:metadata:update":
			handler = m.oplog.Wrap(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "编辑 Workforce 任务信息", IncludeParams: true, IncludeResult: true}, handler)
		case "workforce:open-integration:create":
			// The response contains one-time API and signing secrets. Audit only
			// the safe request metadata and never the result body.
			handler = m.oplog.Wrap(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "创建 Workforce 开放接入应用", IncludeParams: true, IncludeResult: false}, handler)
		case "workforce:open-integration:operate":
			// Enable/disable and credential rotation share the same operator
			// permission. Keep the audit label accurate for both while never
			// recording a one-time credential response.
			handler = m.oplog.Wrap(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "操作 Workforce 开放接入应用", IncludeParams: false, IncludeResult: false}, handler)
		}
	}
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		if !securitycontext.HasPermission(reqCtx, strings.TrimSpace(permission)) {
			response.Error(reqCtx, apperrors.PermissionDenied(permission))
			return
		}
		handler(ctx, reqCtx)
	}
}

func (m *Module) wrapTaskNotificationUpdate(handler app.HandlerFunc) app.HandlerFunc {
	if m.oplog != nil {
		handler = m.oplog.Wrap(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "更新 Workforce 任务通知", IncludeParams: true, IncludeResult: true}, handler)
	}
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		if !securitycontext.IsLogin(reqCtx) {
			response.Error(reqCtx, apperrors.Unauthorized("未登录"))
			return
		}
		if !securitycontext.HasPermission(reqCtx, "workforce:task:query") && !securitycontext.HasPermission(reqCtx, "workforce:task-notification:operate") {
			response.Error(reqCtx, apperrors.PermissionDenied("workforce:task-notification:operate"))
			return
		}
		handler(ctx, reqCtx)
	}
}

type frameworkTaskPublisher struct {
	client *rabbitmqinfra.Client
}

func (p frameworkTaskPublisher) PublishTask(ctx context.Context, event taskinfra.OutboxEvent) error {
	if event.Exchange == "" || event.RoutingKey == "" {
		return fmt.Errorf("targeted outbox routing is required")
	}
	return p.client.PublishRaw(ctx, rabbitmqinfra.RawPublishOptions{
		Exchange: event.Exchange, RoutingKey: event.RoutingKey, MessageID: strconv.FormatInt(event.ID, 10), Body: event.Payload, Expiration: time.Duration(event.ExpirationMS) * time.Millisecond,
	})
}

type workerQueueTopology struct{ client *rabbitmqinfra.Client }

func (p workerQueueTopology) EnsureWorkerQueue(_ context.Context, contract registrydomain.QueueContract) error {
	return p.client.DeclareTopology(rabbitmqinfra.Topology{
		Exchanges: []rabbitmqinfra.ExchangeOptions{{Name: taskExchange, Type: "direct", Durable: true}, {Name: taskDLX, Type: "direct", Durable: true}, {Name: contract.PostprocessExchange, Type: "direct", Durable: true}},
		Queues: []rabbitmqinfra.QueueDeclaration{
			{Name: contract.Queue, Options: rabbitmqinfra.QueueOptions{DeadLetterExchange: taskDLX, DeadLetterRoutingKey: "dead." + contract.Queue, MessageTTL: 2 * time.Minute, MaxLength: 32, Overflow: "reject-publish-dlx"}},
			{Name: contract.DeadQueue},
			{Name: contract.PostprocessQueue, Options: rabbitmqinfra.QueueOptions{DeadLetterExchange: taskDLX, DeadLetterRoutingKey: "dead." + contract.PostprocessQueue, MessageTTL: 10 * time.Minute, MaxLength: 128, Overflow: "reject-publish-dlx"}},
			{Name: contract.PostprocessDeadQueue},
		},
		Bindings: []rabbitmqinfra.Binding{{Exchange: taskExchange, Queue: contract.Queue, RoutingKey: contract.RoutingKey}, {Exchange: taskDLX, Queue: contract.DeadQueue, RoutingKey: "dead." + contract.Queue}, {Exchange: contract.PostprocessExchange, Queue: contract.PostprocessQueue, RoutingKey: contract.PostprocessRoutingKey}, {Exchange: taskDLX, Queue: contract.PostprocessDeadQueue, RoutingKey: "dead." + contract.PostprocessQueue}},
	})
}

func (p workerQueueTopology) DeleteWorkerQueue(_ context.Context, contract registrydomain.QueueContract) (int, error) {
	taskMessages, err := p.client.DeleteQueueBinding(contract.Queue, contract.Exchange, contract.RoutingKey)
	if err != nil {
		return taskMessages, err
	}
	postprocessMessages, err := p.client.DeleteQueueBinding(contract.PostprocessQueue, contract.PostprocessExchange, contract.PostprocessRoutingKey)
	return taskMessages + postprocessMessages, err
}
