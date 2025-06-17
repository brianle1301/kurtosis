package get_running_services

import (
	"context"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/service_network"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/enclave_plan_persistence"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/enclave_structure"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_starlark_framework"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_starlark_framework/builtin_argument"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_starlark_framework/kurtosis_plan_instruction"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/plan_yaml"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_errors"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_validator"
	"go.starlark.net/starlark"
)

const (
	GetServicesBuiltinName = "get_running_services"
	descriptionStr         = "Fetching running services"
)

func NewGetRunningServices(serviceNetwork service_network.ServiceNetwork) *kurtosis_plan_instruction.KurtosisPlanInstruction {
	return &kurtosis_plan_instruction.KurtosisPlanInstruction{
		KurtosisBaseBuiltin: &kurtosis_starlark_framework.KurtosisBaseBuiltin{
			Name:        GetServicesBuiltinName,
			Arguments:   []*builtin_argument.BuiltinArgument{},
			Deprecation: nil,
		},
		Capabilities: func() kurtosis_plan_instruction.KurtosisPlanInstructionCapabilities {
			return &GetRunningServicesCapabilities{
				serviceNetwork: serviceNetwork,
				serviceNames:   []service.ServiceName{}, // populated at interpretation time
				description:    "",                      // populated at interpretation time
			}
		},
		DefaultDisplayArguments: map[string]bool{},
	}
}

type GetRunningServicesCapabilities struct {
	serviceNetwork service_network.ServiceNetwork
	serviceNames   []service.ServiceName
	description    string
}

func (builtin *GetRunningServicesCapabilities) Interpret(_ string, arguments *builtin_argument.ArgumentValuesSet) (starlark.Value, *startosis_errors.InterpretationError) {
	builtin.description = builtin_argument.GetDescriptionOrFallBack(arguments, descriptionStr)

	serviceNames, err := builtin.serviceNetwork.GetServiceNames()
	if err != nil {
		return nil, startosis_errors.WrapWithInterpretationError(err, "An error occurred while fetching service.")
	}

	servicesList := &starlark.List{}
	for name, _ := range serviceNames {
		_ = servicesList.Append(starlark.String(name))
	}

	return servicesList, nil
}

func (builtin *GetRunningServicesCapabilities) Validate(_ *builtin_argument.ArgumentValuesSet, validatorEnvironment *startosis_validator.ValidatorEnvironment) *startosis_errors.ValidationError {
	return nil
}

func (builtin *GetRunningServicesCapabilities) Execute(_ context.Context, _ *builtin_argument.ArgumentValuesSet) (string, error) {
	// note: this is a no op
	return descriptionStr, nil
}

func (builtin *GetRunningServicesCapabilities) TryResolveWith(instructionsAreEqual bool, _ *enclave_plan_persistence.EnclavePlanInstruction, enclaveComponents *enclave_structure.EnclaveComponents) enclave_structure.InstructionResolutionStatus {
	return enclave_structure.InstructionIsNotResolvableAbort
}

func (builtin *GetRunningServicesCapabilities) FillPersistableAttributes(builder *enclave_plan_persistence.EnclavePlanInstructionBuilder) {
	builder.SetType(GetServicesBuiltinName)
}

func (builtin *GetRunningServicesCapabilities) UpdatePlan(planYaml *plan_yaml.PlanYamlGenerator) error {
	// get services does not affect the plan
	return nil
}

func (builtin *GetRunningServicesCapabilities) Description() string {
	return builtin.description
}
