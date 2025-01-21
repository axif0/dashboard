/*
Copyright 2024 The Karmada Authors.

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

package v1

// todo this is only a simple version of pp request, just for POC
type PostPropagationPolicyRequest struct {
	PropagationData string `json:"propagationData" binding:"required"`
	IsClusterScope  bool   `json:"isClusterScope"`
	Namespace       string `json:"namespace"`
}

type PostPropagationPolicyResponse struct {
}

type PutPropagationPolicyRequest struct {
	PropagationData string `json:"propagationData" binding:"required"`
	IsClusterScope  bool   `json:"isClusterScope"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name" binding:"required"`
}

type PutPropagationPolicyResponse struct {
}

type DeletePropagationPolicyRequest struct {
	IsClusterScope bool   `json:"isClusterScope"`
	Namespace      string `json:"namespace"`
	Name           string `json:"name" binding:"required"`
}

type DeletePropagationPolicyResponse struct {
}
