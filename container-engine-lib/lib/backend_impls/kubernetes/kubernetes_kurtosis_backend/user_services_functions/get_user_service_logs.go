package user_services_functions

import (
	"context"
	"io"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/shared_helpers"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_manager"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/enclave"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/stacktrace"
)

const (
	shouldAddTimestampsToUserServiceLogs = false
)

func GetUserServiceLogs(
	ctx context.Context,
	enclaveId enclave.EnclaveUUID,
	filters *service.ServiceFilters,
	shouldFollowLogs bool,
	cliModeArgs *shared_helpers.CliModeArgs,
	apiContainerModeArgs *shared_helpers.ApiContainerModeArgs,
	engineServerModeArgs *shared_helpers.EngineServerModeArgs,
	kubernetesManager *kubernetes_manager.KubernetesManager,
) (successfulUserServiceLogs map[service.ServiceUUID]io.ReadCloser, erroredUserServiceGuids map[service.ServiceUUID]error, resultError error) {
	serviceObjectsAndResources, err := shared_helpers.GetMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, filters, cliModeArgs, apiContainerModeArgs, engineServerModeArgs, kubernetesManager)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "Expected to be able to get user services and Kubernetes resources, instead a non nil error was returned")
	}
	userServiceLogs := map[service.ServiceUUID]io.ReadCloser{}
	erredServiceLogs := map[service.ServiceUUID]error{}
	shouldCloseLogStreams := true
	for _, serviceObjectAndResource := range serviceObjectsAndResources {
		serviceUuid := serviceObjectAndResource.Service.GetRegistration().GetUUID()

		workload := serviceObjectAndResource.KubernetesResources.Workload
		pod, err := workload.GetPod(ctx, kubernetesManager)
		if err != nil {
			return nil, nil, stacktrace.Propagate(err, "An error occurred getting pods managed by %s '%s'", workload.ReadableType(), workload.Name())
		}

		serviceNamespaceName := serviceObjectAndResource.KubernetesResources.Service.GetNamespace()
		// Get logs
		logReadCloser, err := kubernetesManager.GetContainerLogs(ctx, serviceNamespaceName, pod.Name, userServiceContainerName, shouldFollowLogs, shouldAddTimestampsToUserServiceLogs)
		if err != nil {
			erredServiceLogs[serviceUuid] = stacktrace.Propagate(err, "Expected to be able to call Kubernetes to get logs for service with UUID '%v', instead a non-nil error was returned", serviceUuid)
			continue
		}
		defer func() {
			if shouldCloseLogStreams {
				logReadCloser.Close()
			}
		}()

		userServiceLogs[serviceUuid] = logReadCloser
	}

	shouldCloseLogStreams = false
	return userServiceLogs, erredServiceLogs, nil
}
