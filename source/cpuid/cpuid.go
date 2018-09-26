/*
Copyright 2017 The Kubernetes Authors.

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

package cpuid

import (
	"github.com/klauspost/cpuid"
	"github.com/kubernetes-incubator/node-feature-discovery/source"
)

// Source implements FeatureSource.
type Source struct{}

// Name returns an identifier string for this feature source.
func (s Source) Name() string { return "cpuid" }

// Discover returns feature names for all the supported CPU features.
func (s Source) Discover() (source.Features, error) {
	// Get the cpu features as strings
	features := source.Features{}
	for _, f := range cpuid.CPU.Features.Strings() {
		features[f] = true
	}
	return features, nil
}
