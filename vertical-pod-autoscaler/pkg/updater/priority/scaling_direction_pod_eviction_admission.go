/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package priority

import (
	apiv1 "k8s.io/api/core/v1"
	vpa_types "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpa_utils "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/vpa"
)

// NewScalingDirectionPodEvictionAdmission creates a PodEvictionAdmission object.
// It admits Pods for eviction based on the scaling direction, i.e. if a resource is scaled up (recommendation > requests) or scaled down (recommendation < requests).
// It also supports >= and <= relations.
func NewScalingDirectionPodEvictionAdmission() PodEvictionAdmission {
	return &scalingDirectionPodEvictionAdmission{}
}

type scalingDirectionPodEvictionAdmission struct {
	EvictionRequirements map[*apiv1.Pod][]*vpa_types.EvictionRequirement
}

// Admit admits a Pod for eviction in one of three cases
// * an EvictionRequirement exists which is evaluated to 'true' for at least one Container in this Pod
// * no Recommendation exists for at least one Container in this Pod
// * no EvictionRequirement exists for this Pod
func (s *scalingDirectionPodEvictionAdmission) Admit(pod *apiv1.Pod, resources *vpa_types.RecommendedPodResources) bool {
	podEvictionRequirements, found := s.EvictionRequirements[pod]
	if !found {
		return true
	}
	for _, container := range pod.Spec.Containers {
		recommendedResources := vpa_utils.GetRecommendationForContainer(container.Name, resources)
		if recommendedResources == nil {
			return true
		}
		if s.admitContainer(container, recommendedResources, podEvictionRequirements) {
			return true
		}
	}
	return false
}

func (s *scalingDirectionPodEvictionAdmission) admitContainer(container apiv1.Container, recommendedResources *vpa_types.RecommendedContainerResources, podEvictionRequirements []*vpa_types.EvictionRequirement) bool {
	requestedCPU, foundCPURequests := container.Resources.Requests[apiv1.ResourceCPU]
	recommendedCPU := recommendedResources.Target[apiv1.ResourceCPU]
	if foundCPURequests && !s.allowEvictionForResourceAndScalingDirection(apiv1.ResourceCPU, requestedCPU.MilliValue(), recommendedCPU.MilliValue(), podEvictionRequirements) {
		return false
	}

	requestedMemory, foundMemoryRequests := container.Resources.Requests[apiv1.ResourceMemory]
	recommendedMemory := recommendedResources.Target[apiv1.ResourceMemory]
	if foundMemoryRequests && !s.allowEvictionForResourceAndScalingDirection(apiv1.ResourceMemory, requestedMemory.MilliValue(), recommendedMemory.MilliValue(), podEvictionRequirements) {
		return false
	}
	return true
}

func (s *scalingDirectionPodEvictionAdmission) allowEvictionForResourceAndScalingDirection(resource apiv1.ResourceName, currentRequests int64, recommendation int64, podEvictionRequirements []*vpa_types.EvictionRequirement) bool {
	for _, evictionRequirement := range podEvictionRequirements {
		for _, resourceName := range evictionRequirement.Resources {
			if resourceName != resource {
				continue
			}
			if evictionRequirement.ChangeRequirement == vpa_types.TargetHigherThanRequests {
				return recommendation > currentRequests
			}
			if evictionRequirement.ChangeRequirement == vpa_types.TargetLowerThanRequests {
				return recommendation < currentRequests
			}
		}
	}
	return true
}

// LoopInit initializes the object by creating a map holding all applicable EvictionRequirements for each Pod.
// The map is re-created on every call, to ensure that any changes to a VPA's EvictionRequirements are picked up and not leak any EvictionRequirements for no longer existing Pods.
func (s *scalingDirectionPodEvictionAdmission) LoopInit(_ []*apiv1.Pod, vpaControlledPods map[*vpa_types.VerticalPodAutoscaler][]*apiv1.Pod) {
	s.EvictionRequirements = make(map[*apiv1.Pod][]*vpa_types.EvictionRequirement)
	for vpa, pods := range vpaControlledPods {
		for _, pod := range pods {
			s.EvictionRequirements[pod] = vpa.Spec.UpdatePolicy.EvictionRequirements
		}
	}
}

func (s *scalingDirectionPodEvictionAdmission) CleanUp() {
}
