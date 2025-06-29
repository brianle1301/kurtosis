package kubernetes_kurtosis_backend

import (
	"context"
	"io"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/logs_aggregator_functions"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/logs_aggregator_functions/implementations/vector"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/logs_collector_functions"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/logs_collector_functions/implementations/fluentbit"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/container"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/image_build_spec"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/image_registry_spec"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/nix_build_spec"
	apiv1 "k8s.io/api/core/v1"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/engine_functions"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/shared_helpers"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/user_services_functions"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_manager"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/kubernetes_label_key"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/label_value_consts"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/compute_resources"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/enclave"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/engine"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/exec_result"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/image_download_mode"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/logs_aggregator"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/logs_collector"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/reverse_proxy"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/sirupsen/logrus"
)

const (
	isResourceInformationComplete                      = false
	noProductionMode                                   = false
	anyNodeEngineNodeName                              = "" // engine can be scheduled by k8s on any node
	defaultShouldTurnOffPersistentVolumeLogsCollection = false
)

type KubernetesKurtosisBackend struct {
	kubernetesManager *kubernetes_manager.KubernetesManager

	objAttrsProvider object_attributes_provider.KubernetesObjectAttributesProvider

	cliModeArgs *shared_helpers.CliModeArgs

	engineServerModeArgs *shared_helpers.EngineServerModeArgs

	// Will only be filled out for the API container
	apiContainerModeArgs *shared_helpers.ApiContainerModeArgs

	// Whether services should be restarted
	productionMode bool

	// Name of node that engine will get scheduled on via a node selector
	engineNodeName string
}

func (backend *KubernetesKurtosisBackend) DumpKurtosis(ctx context.Context, outputDirpath string) error {
	//TODO implement me
	panic("implement me")
}

// Private constructor that the other public constructors will use
func newKubernetesKurtosisBackend(
	kubernetesManager *kubernetes_manager.KubernetesManager,
	cliModeArgs *shared_helpers.CliModeArgs,
	engineServerModeArgs *shared_helpers.EngineServerModeArgs,
	apiContainerModeArgs *shared_helpers.ApiContainerModeArgs,
	productionMoe bool,
	engineNodeName string,
) *KubernetesKurtosisBackend {
	objAttrsProvider := object_attributes_provider.GetKubernetesObjectAttributesProvider()
	return &KubernetesKurtosisBackend{
		kubernetesManager:    kubernetesManager,
		objAttrsProvider:     objAttrsProvider,
		cliModeArgs:          cliModeArgs,
		engineServerModeArgs: engineServerModeArgs,
		apiContainerModeArgs: apiContainerModeArgs,
		productionMode:       productionMoe,
		engineNodeName:       engineNodeName,
	}
}

func NewAPIContainerKubernetesKurtosisBackend(
	kubernetesManager *kubernetes_manager.KubernetesManager,
	ownEnclaveUuid enclave.EnclaveUUID,
	ownNamespaceName string,
	storageClassName string,
	productionMode bool,
) *KubernetesKurtosisBackend {
	modeArgs := shared_helpers.NewApiContainerModeArgs(ownEnclaveUuid, ownNamespaceName, storageClassName)
	return newKubernetesKurtosisBackend(
		kubernetesManager,
		nil,
		nil,
		modeArgs,
		productionMode,
		anyNodeEngineNodeName,
	)
}

func NewEngineServerKubernetesKurtosisBackend(
	kubernetesManager *kubernetes_manager.KubernetesManager,
) *KubernetesKurtosisBackend {
	modeArgs := &shared_helpers.EngineServerModeArgs{}
	return newKubernetesKurtosisBackend(
		kubernetesManager,
		nil,
		modeArgs,
		nil,
		noProductionMode,
		anyNodeEngineNodeName,
	)
}

func NewCLIModeKubernetesKurtosisBackend(
	kubernetesManager *kubernetes_manager.KubernetesManager,
	engineNodeName string,
) *KubernetesKurtosisBackend {
	modeArgs := &shared_helpers.CliModeArgs{}
	return newKubernetesKurtosisBackend(
		kubernetesManager,
		modeArgs,
		nil,
		nil,
		noProductionMode,
		engineNodeName,
	)
}

func (backend *KubernetesKurtosisBackend) FetchImage(ctx context.Context, image string, registrySpec *image_registry_spec.ImageRegistrySpec, downloadMode image_download_mode.ImageDownloadMode) (bool, string, error) {
	logrus.Warnf("FetchImage isn't implemented for Kubernetes yet")
	return false, "", nil
}

func (backend *KubernetesKurtosisBackend) PruneUnusedImages(ctx context.Context) ([]string, error) {
	logrus.Warnf("PruneUnusedImages isn't implemented for Kubernetes yet")
	return nil, nil
}

func (backend *KubernetesKurtosisBackend) CreateEngine(
	ctx context.Context,
	imageOrgAndRepo string,
	imageVersionTag string,
	grpcPortNum uint16,
	envVars map[string]string,
	shouldStartInDebugMode bool,
	githubAuthToken string,
	sinks logs_aggregator.Sinks,
	shouldEnablePersistentVolumeLogsCollection bool,
	logsCollectorFilters []logs_collector.Filter,
	logsCollectorParsers []logs_collector.Parser,
) (
	*engine.Engine,
	error,
) {
	kubernetesEngine, err := engine_functions.CreateEngine(
		ctx,
		imageOrgAndRepo,
		imageVersionTag,
		grpcPortNum,
		envVars,
		shouldStartInDebugMode,
		githubAuthToken,
		sinks,
		shouldEnablePersistentVolumeLogsCollection,
		logsCollectorFilters,
		logsCollectorParsers,
		backend.engineNodeName,
		backend.kubernetesManager,
		backend.objAttrsProvider,
	)
	if err != nil {
		return nil, stacktrace.Propagate(
			err,
			"An error occurred creating engine using image '%v:%v', grpc port number '%v' and environment variables '%+v'",
			imageOrgAndRepo,
			imageVersionTag,
			grpcPortNum,
			envVars,
		)
	}
	return kubernetesEngine, nil
}

func (backend *KubernetesKurtosisBackend) GetEngines(
	ctx context.Context,
	filters *engine.EngineFilters,
) (map[engine.EngineGUID]*engine.Engine, error) {
	engines, err := engine_functions.GetEngines(ctx, filters, backend.kubernetesManager, backend.engineNodeName)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting engines using filters '%+v'", filters)
	}
	return engines, nil
}

func (backend *KubernetesKurtosisBackend) GetEngineLogs(
	ctx context.Context,
	outputDirpath string,
) error {
	if err := engine_functions.GetEngineLogs(ctx, outputDirpath, backend.kubernetesManager); err != nil {
		return stacktrace.Propagate(err, "An error occurred getting engine logs")
	}
	return nil
}

func (backend *KubernetesKurtosisBackend) StopEngines(
	ctx context.Context,
	filters *engine.EngineFilters,
) (
	resultSuccessfulEngineGuids map[engine.EngineGUID]bool,
	resultErroredEngineGuids map[engine.EngineGUID]error,
	resultErr error,
) {
	successfulEngineGuids, erroredEngineGuids, err := engine_functions.StopEngines(ctx, filters, backend.kubernetesManager, backend.engineNodeName)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred stopping engines using filters '%+v'", filters)
	}
	return successfulEngineGuids, erroredEngineGuids, nil
}

func (backend *KubernetesKurtosisBackend) DestroyEngines(
	ctx context.Context,
	filters *engine.EngineFilters,
) (
	resultSuccessfulEngineGuids map[engine.EngineGUID]bool,
	resultErroredEngineGuids map[engine.EngineGUID]error,
	resultErr error,
) {
	successfulEngineGuids, erroredEngineGuids, err := engine_functions.DestroyEngines(ctx, filters, backend.kubernetesManager, backend.engineNodeName)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred destroying engines using filters '%+v'", filters)
	}
	return successfulEngineGuids, erroredEngineGuids, nil
}

func (backend *KubernetesKurtosisBackend) RegisterUserServices(ctx context.Context, enclaveUuid enclave.EnclaveUUID, services map[service.ServiceName]bool) (map[service.ServiceName]*service.ServiceRegistration, map[service.ServiceName]error, error) {
	successfullyRegisteredService, failedServices, err := user_services_functions.RegisterUserServices(
		ctx,
		enclaveUuid,
		services,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
	if err != nil {
		var serviceIds []service.ServiceName
		for serviceId := range services {
			serviceIds = append(serviceIds, serviceId)
		}
		return nil, nil, stacktrace.Propagate(err, "Unexpected error registering services with Names '%v' to enclave '%s'", serviceIds, enclaveUuid)
	}
	return successfullyRegisteredService, failedServices, nil
}

func (backend *KubernetesKurtosisBackend) UnregisterUserServices(ctx context.Context, enclaveUuid enclave.EnclaveUUID, services map[service.ServiceUUID]bool) (map[service.ServiceUUID]bool, map[service.ServiceUUID]error, error) {
	successfullyUnregisteredServices, failedServices, err := user_services_functions.UnregisterUserServices(
		ctx,
		enclaveUuid,
		services,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
	if err != nil {
		var serviceUuids []service.ServiceUUID
		for serviceUuid := range services {
			serviceUuids = append(serviceUuids, serviceUuid)
		}
		return nil, nil, stacktrace.Propagate(err, "Unexpected error unregistering services with GUIDs '%v' from enclave '%s'", serviceUuids, enclaveUuid)
	}
	return successfullyUnregisteredServices, failedServices, nil
}

func (backend *KubernetesKurtosisBackend) StartRegisteredUserServices(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	services map[service.ServiceUUID]*service.ServiceConfig,
) (
	map[service.ServiceUUID]*service.Service,
	map[service.ServiceUUID]error,
	error,
) {
	restartPolicy := apiv1.RestartPolicyNever
	if backend.productionMode {
		restartPolicy = apiv1.RestartPolicyAlways
	}

	successfullyStartedServices, failedServices, err := user_services_functions.StartRegisteredUserServices(
		ctx,
		enclaveUuid,
		services,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager,
		restartPolicy)
	if err != nil {
		var serviceUuids []service.ServiceUUID
		for serviceUuid := range services {
			serviceUuids = append(serviceUuids, serviceUuid)
		}
		return nil, nil, stacktrace.Propagate(err, "Unexpected error starting services with GUIDs '%v' in enclave '%s'", serviceUuids, enclaveUuid)
	}
	return successfullyStartedServices, failedServices, nil
}

func (backend *KubernetesKurtosisBackend) RemoveRegisteredUserServiceProcesses(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	services map[service.ServiceUUID]bool,
) (
	map[service.ServiceUUID]bool,
	map[service.ServiceUUID]error,
	error,
) {
	successfullyStartedServices, failedServices, err := user_services_functions.RemoveRegisteredUserServiceProcesses(
		ctx,
		enclaveUuid,
		services,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
	if err != nil {
		var serviceUuids []service.ServiceUUID
		for serviceUuid := range services {
			serviceUuids = append(serviceUuids, serviceUuid)
		}
		return nil, nil, stacktrace.Propagate(err, "Unexpected error removing services with GUIDs '%v' in enclave '%s'", serviceUuids, enclaveUuid)
	}
	return successfullyStartedServices, failedServices, nil
}

func (backend *KubernetesKurtosisBackend) GetUserServices(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	filters *service.ServiceFilters,
) (successfulUserServices map[service.ServiceUUID]*service.Service, resultError error) {
	return user_services_functions.GetUserServices(
		ctx,
		enclaveUuid,
		filters,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) GetUserServiceLogs(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	filters *service.ServiceFilters,
	shouldFollowLogs bool,
) (successfulUserServiceLogs map[service.ServiceUUID]io.ReadCloser, erroredUserServiceUuids map[service.ServiceUUID]error, resultError error) {
	return user_services_functions.GetUserServiceLogs(
		ctx,
		enclaveUuid,
		filters,
		shouldFollowLogs,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) RunUserServiceExecCommands(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	containerUser string,
	userServiceCommands map[service.ServiceUUID][]string,
) (
	succesfulUserServiceExecResults map[service.ServiceUUID]*exec_result.ExecResult,
	erroredUserServiceUuids map[service.ServiceUUID]error,
	resultErr error,
) {
	if containerUser != "" {
		resultErr = stacktrace.NewError("--user not implemented for kurtosis backend")
		return
	}
	return user_services_functions.RunUserServiceExecCommands(
		ctx,
		enclaveUuid,
		userServiceCommands,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) RunUserServiceExecCommandWithStreamedOutput(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	serviceUuid service.ServiceUUID,
	cmd []string,
) (chan string, chan *exec_result.ExecResult, error) {
	return user_services_functions.RunUserServiceExecCommandWithStreamedOutput(
		ctx,
		enclaveUuid,
		serviceUuid,
		cmd,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) GetShellOnUserService(ctx context.Context, enclaveUuid enclave.EnclaveUUID, serviceUuid service.ServiceUUID) (resultErr error) {
	objectAndResources, err := shared_helpers.GetSingleUserServiceObjectsAndResources(ctx, enclaveUuid, serviceUuid, backend.cliModeArgs, backend.apiContainerModeArgs, backend.engineServerModeArgs, backend.kubernetesManager)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting user service object & Kubernetes resources for service '%v' in enclave '%v'", serviceUuid, enclaveUuid)
	}

	workload := objectAndResources.KubernetesResources.Workload
	pod, err := workload.GetPod(ctx, backend.kubernetesManager)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting pods managed by %s '%s'", workload.ReadableType(), workload.Name())
	}

	return backend.kubernetesManager.GetExecStream(ctx, pod)
}

func (backend *KubernetesKurtosisBackend) CopyFilesFromUserService(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	serviceUuid service.ServiceUUID,
	srcPath string,
	output io.Writer,
) error {
	return user_services_functions.CopyFilesFromUserService(
		ctx,
		enclaveUuid,
		serviceUuid,
		srcPath,
		output,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) StopUserServices(ctx context.Context, enclaveUuid enclave.EnclaveUUID, filters *service.ServiceFilters) (resultSuccessfulGuids map[service.ServiceUUID]bool, resultErroredGuids map[service.ServiceUUID]error, resultErr error) {
	return user_services_functions.StopUserServices(
		ctx,
		enclaveUuid,
		filters,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) DestroyUserServices(ctx context.Context, enclaveUuid enclave.EnclaveUUID, filters *service.ServiceFilters) (resultSuccessfulGuids map[service.ServiceUUID]bool, resultErroredGuids map[service.ServiceUUID]error, resultErr error) {
	return user_services_functions.DestroyUserServices(
		ctx,
		enclaveUuid,
		filters,
		backend.cliModeArgs,
		backend.apiContainerModeArgs,
		backend.engineServerModeArgs,
		backend.kubernetesManager)
}

func (backend *KubernetesKurtosisBackend) GetAvailableCPUAndMemory(ctx context.Context) (compute_resources.MemoryInMegaBytes, compute_resources.CpuMilliCores, bool, error) {
	// TODO - implement resource calculation in kubernetes
	return 0, 0, isResourceInformationComplete, nil
}

func (backend *KubernetesKurtosisBackend) GetLogsAggregator(
	ctx context.Context,
) (*logs_aggregator.LogsAggregator, error) {
	maybeLogsAggregator, err := logs_aggregator_functions.GetLogsAggregator(
		ctx,
		backend.kubernetesManager,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the logs aggregator")
	}
	return maybeLogsAggregator, nil
}

func (backend *KubernetesKurtosisBackend) CreateLogsAggregator(ctx context.Context, httpPortNum uint16, sinks logs_aggregator.Sinks) (*logs_aggregator.LogsAggregator, error) {
	logsAggregatorDeployment := vector.NewVectorLogsAggregatorResourcesManager()

	logsAggregator, _, err := logs_aggregator_functions.CreateLogsAggregator(
		ctx,
		"", // as of now, nothing calls this functions so it's okay to leave namespace blank
		logsAggregatorDeployment,
		httpPortNum,
		sinks,
		defaultShouldTurnOffPersistentVolumeLogsCollection,
		backend.objAttrsProvider,
		backend.kubernetesManager)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating logs aggregator.")
	}

	return logsAggregator, nil
}

func (backend *KubernetesKurtosisBackend) DestroyLogsAggregator(ctx context.Context) error {
	if err := logs_aggregator_functions.DestroyLogsAggregator(ctx, backend.kubernetesManager); err != nil {
		return stacktrace.Propagate(err, "An error occurred destroying logs aggregator.")
	}
	logrus.Debug("Successfully destroyed logs aggregator.")
	return nil
}

func (backend *KubernetesKurtosisBackend) CreateLogsCollectorForEnclave(
	ctx context.Context,
	enclaveUuid enclave.EnclaveUUID,
	logsCollectorHttpPortNumber uint16,
	logsCollectorTcpPortNumber uint16,
	logsCollectorFilters []logs_collector.Filter,
	logsCollectorParsers []logs_collector.Parser,
) (
	*logs_collector.LogsCollector,
	error,
) {
	var logsAggregator *logs_aggregator.LogsAggregator
	maybeLogsAggregator, err := logs_aggregator_functions.GetLogsAggregator(ctx, backend.kubernetesManager)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the logs aggregator. The logs collector cannot be run without a logs aggregator.")
	}
	if maybeLogsAggregator == nil {
		logrus.Warnf("Logs aggregator does not exist. This is unexpected as Kubernetes should have restarted the deployment automatically.")
		logrus.Warnf("This can be fixed by restarting the engine using `kurto engine restart` and attempting to create the enclave again.")
		return nil, stacktrace.NewError("No logs aggregator exists. The logs collector cannot be run without a logs aggregator.")
	}
	if maybeLogsAggregator.GetStatus() != container.ContainerStatus_Running {
		logrus.Warnf("Logs aggregator exists but is not running. Instead status is '%v'. This is unexpected as k8s should have restarted the aggregator automatically.",
			maybeLogsAggregator.GetStatus())
		logrus.Warnf("This can be fixed by restarting the engine using `kurtosis engine restart` and attempting to create the enclave again.")
		return nil, stacktrace.NewError(
			"The logs aggregator deployment exists but is not running. Instead logs aggregator status is '%v'. The logs collector cannot be run without a logs aggregator.",
			maybeLogsAggregator.GetStatus(),
		)
	}
	logsAggregator = maybeLogsAggregator

	//Declaring the implementation
	logsCollectorDaemonSet := fluentbit.NewFluentbitLogsCollector()

	logrus.Info("Creating logs collector...")
	logsCollector, _, err := logs_collector_functions.CreateLogsCollector(
		ctx,
		logsCollectorTcpPortNumber,
		logsCollectorHttpPortNumber,
		logsCollectorDaemonSet,
		logsAggregator,
		logsCollectorFilters,
		logsCollectorParsers,
		backend.kubernetesManager,
		backend.objAttrsProvider,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating the logs collector using the '%v' TCP port number, the '%v' HTTP port number and the logs collector daemon set '%+v'", logsCollectorTcpPortNumber, logsCollectorHttpPortNumber, logsCollectorDaemonSet)
	}

	return logsCollector, nil
}

func (backend *KubernetesKurtosisBackend) GetLogsCollectorForEnclave(ctx context.Context, enclaveUuid enclave.EnclaveUUID) (*logs_collector.LogsCollector, error) {
	maybeLogsCollector, err := logs_collector_functions.GetLogsCollector(
		ctx,
		backend.kubernetesManager,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the logs collector")
	}
	return maybeLogsCollector, nil
}

func (backend *KubernetesKurtosisBackend) DestroyLogsCollectorForEnclave(ctx context.Context, enclaveUuid enclave.EnclaveUUID) error {
	if err := logs_collector_functions.DestroyLogsCollector(ctx, backend.kubernetesManager); err != nil {
		return stacktrace.Propagate(err, "An error occurred destroying logs collector.")
	}
	logrus.Debug("Successfully destroyed logs collector.")
	return nil
}

func (backend *KubernetesKurtosisBackend) GetReverseProxy(
	ctx context.Context,
) (*reverse_proxy.ReverseProxy, error) {
	// TODO IMPLEMENT
	return nil, stacktrace.NewError("Getting the reverse proxy isn't yet implemented on Kubernetes")
}

func (backend *KubernetesKurtosisBackend) CreateReverseProxy(ctx context.Context, engineGuid engine.EngineGUID) (*reverse_proxy.ReverseProxy, error) {
	// TODO IMPLEMENT
	return nil, stacktrace.NewError("Creating the reverse proxy isn't yet implemented on Kubernetes")
}

func (backend *KubernetesKurtosisBackend) DestroyReverseProxy(ctx context.Context) error {
	// TODO IMPLEMENT
	return stacktrace.NewError("Destroying the reverse proxy isn't yet implemented on Kubernetes")
}

func (backend *KubernetesKurtosisBackend) BuildImage(ctx context.Context, imageName string, imageBuildSpec *image_build_spec.ImageBuildSpec) (string, error) {
	// TODO IMPLEMENT
	return "", stacktrace.NewError("Building images isn't yet implemented in Kubernetes.")
}

func (backend *KubernetesKurtosisBackend) NixBuild(ctx context.Context, nixBuildSpec *nix_build_spec.NixBuildSpec) (string, error) {
	// TODO IMPLEMENT
	return "", stacktrace.NewError("Nix image building isn't yet implemented in Kubernetes.")
}

// ====================================================================================================
//
//	Private Helper Functions
//
// ====================================================================================================
func (backend *KubernetesKurtosisBackend) getEnclaveNamespaceName(ctx context.Context, enclaveUuid enclave.EnclaveUUID) (string, error) {
	// TODO This is a big janky hack that results from *KubernetesKurtosisBackend containing functions for all of API containers, engines, and CLIs
	//  We want to fix this by splitting the *KubernetesKurtosisBackend into a bunch of different backends, one per user, but we can only
	//  do this once the CLI no longer uses API container functionality (e.g. GetServices)
	// CLIs and engines can list namespaces so they'll be able to use the regular list-namespaces-and-find-the-one-matching-the-enclave-ID
	// API containers can't list all namespaces due to being namespaced objects themselves (can only view their own namespace, so
	// they can only check if the requested enclave matches the one they have stored
	var namespaceName string
	if backend.cliModeArgs != nil || backend.engineServerModeArgs != nil {
		matchLabels := getEnclaveMatchLabels()
		matchLabels[kubernetes_label_key.EnclaveUUIDKubernetesLabelKey.GetString()] = string(enclaveUuid)

		namespaces, err := backend.kubernetesManager.GetNamespacesByLabels(ctx, matchLabels)
		if err != nil {
			return "", stacktrace.Propagate(err, "An error occurred getting the enclave namespace using labels '%+v'", matchLabels)
		}

		numOfNamespaces := len(namespaces.Items)
		if numOfNamespaces == 0 {
			return "", stacktrace.NewError("No namespace matching labels '%+v' was found", matchLabels)
		}
		if numOfNamespaces > 1 {
			return "", stacktrace.NewError("Expected to find only one enclave namespace matching enclave ID '%v', but found '%v'; this is a bug in Kurtosis", enclaveUuid, numOfNamespaces)
		}

		namespaceName = namespaces.Items[0].Name
	} else if backend.apiContainerModeArgs != nil {
		if enclaveUuid != backend.apiContainerModeArgs.GetOwnEnclaveId() {
			return "", stacktrace.NewError(
				"Received a request to get namespace for enclave '%v', but the Kubernetes Kurtosis backend is running in an API "+
					"container in a different enclave '%v' (so Kubernetes would throw a permission error)",
				enclaveUuid,
				backend.apiContainerModeArgs.GetOwnEnclaveId(),
			)
		}
		namespaceName = backend.apiContainerModeArgs.GetOwnNamespaceName()
	} else {
		return "", stacktrace.NewError("Received a request to get an enclave namespace's name, but the Kubernetes Kurtosis backend isn't in any recognized mode; this is a bug in Kurtosis")
	}

	return namespaceName, nil
}

func getEnclaveMatchLabels() map[string]string {
	matchLabels := map[string]string{
		kubernetes_label_key.AppIDKubernetesLabelKey.GetString():                label_value_consts.AppIDKubernetesLabelValue.GetString(),
		kubernetes_label_key.KurtosisResourceTypeKubernetesLabelKey.GetString(): label_value_consts.EnclaveKurtosisResourceTypeKubernetesLabelValue.GetString(),
	}
	return matchLabels
}
