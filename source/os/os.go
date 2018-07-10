/*
Copyright 2018 The Kubernetes Authors.

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

package os_release

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/node-feature-discovery/source"
)

var osReleaseFields = [...]string{
	"ID",
	"VERSION_ID",
}

// Implement FeatureSource interface
type Source struct{}

func (s Source) Name() string { return "os" }

func (s Source) Discover() (source.Features, error) {
	features := source.Features{}

	release, err := parseOSRelease()
	if err != nil {
		glog.Errorf("Failed to get os-release: %v", err)
	} else {
		for _, key := range osReleaseFields {
			if value, exists := release[key]; exists {
				features["release."+key] = value
			}
		}
	}
	return features, nil
}

// Read and parse os-release file
func parseOSRelease() (map[string]string, error) {
	release := map[string]string{}

	f, err := os.Open("/host-etc/os-release")
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`^(?P<key>\w+)=(?P<value>.+)`)

	// Read line-by-line
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if m := re.FindStringSubmatch(line); m != nil {
			release[m[1]] = strings.Trim(m[2], `"`)
		}
	}

	return release, nil
}
