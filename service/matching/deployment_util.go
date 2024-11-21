package matching

import (
	"slices"

	deploymentpb "go.temporal.io/api/deployment/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/matchingservice/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	hlc "go.temporal.io/server/common/clock/hybrid_logical_clock"
)

var (
	errMissingDeployment      = serviceerror.NewInvalidArgument("missing deployment")
	errMissingFirstPollerTime = serviceerror.NewInvalidArgument("missing first poller time")
	errDeploymentNotFound     = serviceerror.NewInvalidArgument("deployment not found")
	errBadDeploymentUpdate    = serviceerror.NewInvalidArgument("bad deployment update type")
)

func findDeployment(deployments []*persistencespb.DeploymentData_Deployment, deployment *deploymentpb.Deployment) int {
	for i, d := range deployments {
		if d.Deployment.SeriesName == deployment.SeriesName && d.Deployment.BuildId == deployment.BuildId {
			return i
		}
	}
	return -1
}

func findCurrentDeployment(deployments []*persistencespb.DeploymentData_Deployment) int {
	maxCurrentIndex := -1
	maxCurrentTime := hlc.Zero(0)
	for i, d := range deployments {
		if hlc.Greater(d.LastBecameCurrentTime, maxCurrentTime) {
			maxCurrentIndex, maxCurrentTime = i, d.LastBecameCurrentTime
		}
	}
	return maxCurrentIndex
}

func registerDeployment(
	prevDeployments []*persistencespb.DeploymentData_Deployment,
	req *matchingservice.UpdateDeploymentUserDataRequest_Register,
) ([]*persistencespb.DeploymentData_Deployment, error) {
	if req.Deployment == nil {
		return nil, errMissingDeployment
	} else if req.FirstPollerTime == nil {
		return nil, errMissingFirstPollerTime
	}

	// check if it's there already
	if findDeployment(prevDeployments, req.Deployment) >= 0 {
		return nil, errUserDataUnmodified
	}

	// need to add it
	newDeployments := slices.Clone(prevDeployments)
	newDeployments = append(newDeployments, &persistencespb.DeploymentData_Deployment{
		Deployment:      req.Deployment,
		FirstPollerTime: req.FirstPollerTime,
	})
	return newDeployments, nil
}

func makeCurrentDeployment(
	prevDeployments []*persistencespb.DeploymentData_Deployment,
	req *matchingservice.UpdateDeploymentUserDataRequest_MakeCurrent,
	now *hlc.Clock,
) ([]*persistencespb.DeploymentData_Deployment, error) {
	if req.Deployment == nil {
		return nil, errMissingDeployment
	}

	foundIndex := findDeployment(prevDeployments, req.Deployment)
	if foundIndex == -1 {
		return nil, errDeploymentNotFound
	}
	currentIndex := findCurrentDeployment(prevDeployments)
	if foundIndex == currentIndex {
		// deployment is already current
		return nil, errUserDataUnmodified
	}

	newDeployments := slices.Clone(prevDeployments)
	newDeployments[foundIndex].LastBecameCurrentTime = now
	return newDeployments, nil
}
