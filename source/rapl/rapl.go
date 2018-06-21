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

package rapl

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/kubernetes-incubator/node-feature-discovery/source"
)

const (
	// RAPL MSR numbers needed by us
	MSR_RAPL_POWER_UNIT = 0x00000606
	MSR_PKG_POWER_INFO  = 0x00000614

	// Bit masks for the MSRs
	POWER_UNIT_POWER_UNITS        = 0x7    // 3 LSBs
	POWER_INFO_THERMAL_SPEC_POWER = 0x3FFF // 14 LSBs

)

// Implement FeatureSource interface
type Source struct{}

func (s Source) Name() string { return "rapl" }

func (s Source) Discover() (source.Features, error) {
	features := source.Features{}

	p, err := thermalSpecPower()
	if err != nil {
		return nil, fmt.Errorf("Failed to detect thermal spec power: %v", err)
	}
	features["thermal-spec-power"] = p

	return features, nil
}

func read_msr(msr int64) (uint64, error) {
	// Simply read from the first logical CPU
	f, err := os.Open("/dev/cpu/0/msr")
	if err != nil {
		return 0, err
	}

	// Seek is used to access the specified MSR
	_, err = f.Seek(msr, 0)
	if err != nil {
		return 0, err
	}

	// Read MSR contents
	buf := make([]byte, 8)
	n, err := f.Read(buf)
	if err != nil {
		return 0, err
	} else if n != 8 {
		err = fmt.Errorf("short read on MSR 0x%x, %v of 8 bytes read", n)
		return 0, err
	}

	// Convert byte slice to uint64
	return binary.LittleEndian.Uint64(buf), nil
}

func thermalSpecPower() (uint64, error) {
	// Read power units
	unitsBits, err := read_msr(MSR_RAPL_POWER_UNIT)
	if err != nil {
		return 0, err
	}
	unitsBits = unitsBits & POWER_UNIT_POWER_UNITS
	units := 1 / float64((uint64(1) << unitsBits))

	// Calculate spec power
	specPowerBits, err := read_msr(MSR_PKG_POWER_INFO)
	if err != nil {
		return 0, err
	}
	specPowerBits = specPowerBits & POWER_INFO_THERMAL_SPEC_POWER

	return uint64(float64(specPowerBits) * units), nil
}
