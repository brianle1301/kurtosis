package user_services_functions

import (
	"context"
	"io"
	"reflect"
	"time"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_kurtosis_backend/shared_helpers"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_manager"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/enclave"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/exec_result"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/operation_parallelizer"
	"github.com/kurtosis-tech/stacktrace"
)

const (
	jobPollInterval = 300 * time.Millisecond
)

func GetUserServiceOutputAndExitCode(
	ctx context.Context,
	enclaveId enclave.EnclaveUUID,
	filters *service.ServiceFilters,
	timeout time.Duration,
	cliModeArgs *shared_helpers.CliModeArgs,
	apiContainerModeArgs *shared_helpers.ApiContainerModeArgs,
	engineServerModeArgs *shared_helpers.EngineServerModeArgs,
	kubernetesManager *kubernetes_manager.KubernetesManager,
) (
	succesfulUserServiceExecResults map[service.ServiceUUID]*exec_result.ExecResult,
	erroredUserServiceGuids map[service.ServiceUUID]error,
	resultErr error,
) {
	serviceObjectsAndResources, err := shared_helpers.GetMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, filters, cliModeArgs, apiContainerModeArgs, engineServerModeArgs, kubernetesManager)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "Expected to be able to get user services and Kubernetes resources, instead a non nil error was returned")
	}

	return getOutputAndExitCodeInParallel(ctx, serviceObjectsAndResources, timeout, kubernetesManager)
}

func getOutputAndExitCodeInParallel(ctx context.Context, objectsAndResources map[service.ServiceUUID]*shared_helpers.UserServiceObjectsAndKubernetesResources, timeout time.Duration, kubernetesManager *kubernetes_manager.KubernetesManager) (map[service.ServiceUUID]*exec_result.ExecResult, map[service.ServiceUUID]error, error) {
	successfulExecs := map[service.ServiceUUID]*exec_result.ExecResult{}
	failedExecs := map[service.ServiceUUID]error{}

	ops := map[operation_parallelizer.OperationID]operation_parallelizer.Operation{}

	for _, serviceObjectAndResource := range objectsAndResources {
		serviceUuid := serviceObjectAndResource.Service.GetRegistration().GetUUID()
		opID := operation_parallelizer.OperationID(serviceUuid)
		op := createOperation(ctx, serviceUuid, serviceObjectAndResource, timeout, kubernetesManager)

		ops[opID] = op
	}

	successfulOperations, failedOperations := operation_parallelizer.RunOperationsInParallel(ops)
	for operationUuid, operationResult := range successfulOperations {
		serviceUuid := service.ServiceUUID(operationUuid)
		execResult, ok := operationResult.(*exec_result.ExecResult)
		if !ok {
			return nil, nil, stacktrace.NewError("An error occurred processing the result of the exec command "+
				"run on service '%s'. It seems the result object is of an unexpected type ('%v'). This is a Kurtosis "+
				"internal bug.", serviceUuid, reflect.TypeOf(execResult))
		}
		successfulExecs[serviceUuid] = execResult
	}
	for operationId, err := range failedOperations {
		serviceUuid := service.ServiceUUID(operationId)
		failedExecs[serviceUuid] = err
	}

	return successfulExecs, failedExecs, nil
}

func createOperation(ctx context.Context, serviceID service.ServiceUUID, objectAndResource *shared_helpers.UserServiceObjectsAndKubernetesResources, timeout time.Duration, kubernetesManager *kubernetes_manager.KubernetesManager) operation_parallelizer.Operation {
	return func() (interface{}, error) {
		workload := objectAndResource.KubernetesResources.Workload
		jw, ok := workload.(*shared_helpers.JobWorkload)
		if !ok {
			return nil, stacktrace.NewError("Can only get output and exit code on Job workloads, but got %s for workload '%s'", workload.ReadableType(), workload.Name())
		}

		if err := jw.WaitForCompletion(ctx, kubernetesManager, jobPollInterval, timeout); err != nil {
			return nil, stacktrace.Propagate(err, "Failed waiting for job to complete")
		}

		pod, err := workload.GetPod(ctx, kubernetesManager)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred getting pods managed by %s '%s'", workload.ReadableType(), workload.Name())
		}

		exitCode := pod.Status.ContainerStatuses[0].State.Terminated.ExitCode

		// Get logs
		logReadCloser, err := kubernetesManager.GetContainerLogs(ctx, pod.Namespace, pod.Name, userServiceContainerName, false, shouldAddTimestampsToUserServiceLogs)
		if err != nil {
			return nil, stacktrace.Propagate(err, "Expected to be able to call Kubernetes to get logs for service with UUID '%v', instead a non-nil error was returned", serviceID)
		}
		defer logReadCloser.Close()

		outputBytes, err := io.ReadAll(logReadCloser)
		if err != nil {
			return nil, stacktrace.Propagate(
				err,
				"Failed to read container logs for pod %s in namespace %s",
				pod.Name,
				pod.Namespace,
			)
		}

		return exec_result.NewExecResult(exitCode, string(outputBytes)), nil
	}
}
