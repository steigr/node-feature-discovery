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

package memory

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/node-feature-discovery/source"
)

// Source implements FeatureSource.
type Source struct{}

// Name returns an identifier string for this feature source.
func (s Source) Name() string { return "memory" }

// Discover returns feature names for memory: numa if more than one memory node is present.
func (s Source) Discover() (source.Features, error) {
	features := source.Features{}

	// Find out how many nodes are online
	// Multiple nodes is a sign of NUMA
	bytes, err := ioutil.ReadFile("/sys/devices/system/node/online")
	if err != nil {
		glog.Errorf("can't read /sys/devices/system/node/online: %s", err.Error())
	} else {
		// File content is expected to be:
		//   "0\n" in one-node case
		//   "0-K\n" in N-node case where K=N-1
		// presence of newline requires TrimSpace
		if strings.TrimSpace(string(bytes)) != "0" {
			// more than one node means NUMA
			features["numa"] = true
		}
	}

	// Try to determine if "die-clustering", i.e. Cluster-on-Die or sub-NUMA
	// clustering has been enabled
	// NOTE: This is indirect guessing, and thus, inherently fragile. E.g.
	// Linux NUMA emulation will fool it, and, there's no guarantee that it
	// will work correctly on future CPUs.
	nodeCount, err := countNodes()
	if err != nil {
		glog.Errorf("Failed to read the number of NUMA nodes: %v", err)
	}
	physicalCount, err := countPhysicalIDs()
	if err != nil {
		glog.Errorf("Failed to read the number of physical (cpu) ids: %v", err)
	}
	if nodeCount > 0 && physicalCount > 0 {
		glog.Errorf("Detected %v NUMA node(s) and %v Physical ID(s)", nodeCount, physicalCount)
		if nodeCount > physicalCount {
			features["die_clustering"] = true
		}
	}

	return features, nil
}

func countNodes() (int, error) {
	files, err := ioutil.ReadDir("/sys/devices/system/node/")
	if err != nil {
		return 0, err
	}

	nodeCount := 0
	re := regexp.MustCompile(`^node[\d]+`)
	for _, file := range files {
		if m := re.MatchString(file.Name()); m == true && file.Mode().IsDir() {
			nodeCount++
		}
	}
	return nodeCount, nil
}

func countPhysicalIDs() (int, error) {
	// Read cpuinfo, line by line
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0, err
	}

	s := bufio.NewScanner(f)
	re := regexp.MustCompile(`^physical id\s+:\s+(\d+)`)
	ids := map[string]bool{}
	for s.Scan() {
		line := s.Text()
		if m := re.FindStringSubmatch(line); m != nil {
			ids[m[1]] = true
		}
	}
	if err := s.Err(); err != nil {
		return 0, err
	}

	// Calculate the number of unique IDs
	idCount := len(ids)
	if idCount == 0 {
		return 0, fmt.Errorf("Failed to parse physical ids from /proc/cpuinfo")
	}
	return idCount, nil
}
