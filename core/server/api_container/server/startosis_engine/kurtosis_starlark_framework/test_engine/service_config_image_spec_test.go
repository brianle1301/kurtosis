package test_engine

import (
	"fmt"
	"testing"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/image_download_mode"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/image_registry_spec"

	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/port_spec"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/service_network"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_starlark_framework/builtin_argument"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_types/service_config"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_packages"
	"github.com/stretchr/testify/require"
)

type serviceConfigImageSpecTest struct {
	*testing.T
	serviceNetwork         *service_network.MockServiceNetwork
	packageContentProvider *startosis_packages.MockPackageContentProvider
}

func (suite *KurtosisTypeConstructorTestSuite) TestServiceConfigWithImageSpecWithRegistry() {
	suite.run(&serviceConfigImageSpecTest{
		T:                      suite.T(),
		serviceNetwork:         suite.serviceNetwork,
		packageContentProvider: suite.packageContentProvider,
	})
}

func (t *serviceConfigImageSpecTest) GetStarlarkCode() string {
	imageSpec := fmt.Sprintf("%s(%s=%q, %s=%q, %s=%q, %s=%q)",
		service_config.ImageSpecTypeName,
		service_config.ImageAttr,
		testContainerImageName,
		service_config.ImageRegistryAttr,
		testRegistryAddr,
		service_config.ImageRegistryUsernameAttr,
		testRegistryUsername,
		service_config.ImageRegistryPasswordAttr,
		testRegistryPassword,
	)
	return fmt.Sprintf("%s(%s=%s)",
		service_config.ServiceConfigTypeName,
		service_config.ImageAttr, imageSpec)
}

func (t *serviceConfigImageSpecTest) Assert(typeValue builtin_argument.KurtosisValueType) {
	serviceConfigStarlark, ok := typeValue.(*service_config.ServiceConfig)
	require.True(t, ok)

	serviceConfig, interpretationErr := serviceConfigStarlark.ToKurtosisType(
		t.serviceNetwork,
		testModuleMainFileLocator,
		testModulePackageId,
		t.packageContentProvider,
		testNoPackageReplaceOptions,
		image_download_mode.ImageDownloadMode_Missing)
	require.Nil(t, interpretationErr)

	expectedImageRegistrySpec := image_registry_spec.NewImageRegistrySpec(testContainerImageName, testRegistryUsername, testRegistryPassword, testRegistryAddr)
	expectedServiceConfig, err := service.CreateServiceConfig(testContainerImageName, nil, expectedImageRegistrySpec, nil, map[string]*port_spec.PortSpec{}, map[string]*port_spec.PortSpec{}, nil, nil, map[string]string{}, nil, nil, 0, 0, service_config.DefaultPrivateIPAddrPlaceholder, 0, 0, map[string]string{}, nil, nil, map[string]string{}, image_download_mode.ImageDownloadMode_Missing, true, service.WorkloadTypeStatefulSet)
	require.NoError(t, err)
	require.Equal(t, expectedServiceConfig, serviceConfig)
	require.Equal(t, expectedImageRegistrySpec, serviceConfig.GetImageRegistrySpec())
}
