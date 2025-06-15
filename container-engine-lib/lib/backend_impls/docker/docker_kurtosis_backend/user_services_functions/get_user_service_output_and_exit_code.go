package user_service_functions

import (
	"context"
	"io"
	"reflect"
	"time"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/docker/docker_kurtosis_backend/shared_helpers"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_impls/docker/docker_manager"
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
	dockerManager *docker_manager.DockerManager,
) (
	successfulUserServiceExecResults map[service.ServiceUUID]*exec_result.ExecResult,
	erroredUserServiceGuids map[service.ServiceUUID]error,
	resultErr error,
) {
	_, serviceObjectsAndResources, err := shared_helpers.GetMatchingUserServiceObjsAndDockerResourcesNoMutex(ctx, enclaveId, filters, dockerManager)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "Expected to be able to get user services and Kubernetes resources, instead a non nil error was returned")
	}

	return getOutputAndExitCodeInParallel(ctx, serviceObjectsAndResources, timeout, dockerManager)
}

func getOutputAndExitCodeInParallel(ctx context.Context, objectsAndResources map[service.ServiceUUID]*shared_helpers.UserServiceDockerResources, timeout time.Duration, dockerManager *docker_manager.DockerManager) (map[service.ServiceUUID]*exec_result.ExecResult, map[service.ServiceUUID]error, error) {
	successfulExecs := map[service.ServiceUUID]*exec_result.ExecResult{}
	failedExecs := map[service.ServiceUUID]error{}

	ops := map[operation_parallelizer.OperationID]operation_parallelizer.Operation{}

	for serviceUuid, serviceObjectAndResource := range objectsAndResources {
		opID := operation_parallelizer.OperationID(serviceUuid)
		op := createOperation(ctx, serviceUuid, serviceObjectAndResource, timeout, dockerManager)

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

func createOperation(ctx context.Context, serviceID service.ServiceUUID, objectAndResource *shared_helpers.UserServiceDockerResources, timeout time.Duration, dockerManager *docker_manager.DockerManager) operation_parallelizer.Operation {
	return func() (interface{}, error) {
		if timeout != 0 {
			deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			ctx = deadlineCtx
		}

		exitCode, err := dockerManager.WaitForExit(ctx, objectAndResource.ServiceContainer.GetId())
		if err != nil {
			return nil, stacktrace.Propagate(err, "Failed waiting for job to complete")
		}

		// Get logs
		logReadCloser, err := dockerManager.GetContainerLogs(ctx, objectAndResource.ServiceContainer.GetId(), false)
		if err != nil {
			return nil, stacktrace.Propagate(err, "Expected to be able to call Docker to get logs for service with UUID '%v', instead a non-nil error was returned", serviceID)
		}
		defer logReadCloser.Close()

		outputBytes, err := io.ReadAll(logReadCloser)
		if err != nil {
			return nil, stacktrace.Propagate(
				err,
				"Failed to read container logs for container %s",
				objectAndResource.ServiceContainer.GetName(),
			)
		}

		return exec_result.NewExecResult(int32(exitCode), string(outputBytes)), nil
	}
}
