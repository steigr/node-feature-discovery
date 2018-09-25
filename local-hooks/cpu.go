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

package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
)

const (
	// MSR numbers needed by us
	MSR_PKG_CST_CONFIG_CONTROL = 0x0E2
	MSR_RAPL_POWER_UNIT        = 0x606
	MSR_PKG_POWER_LIMIT        = 0x610
	MSR_PKG_POWER_INFO         = 0x614
	MSR_UNCORE_RATIO_LIMIT     = 0x620

	// Bit masks for the MSRs
	PKG_CST_CONFIG_CONTROL_LIMIT      = 0x0007         // Bits 0-2
	RAPL_POWER_UNIT_POWER_UNITS       = 0x0007         // Bits 0-2
	PKG_POWER_INFO_THERMAL_SPEC_POWER = 0x7FFF         // Bits 0-14
	PKG_POWER_LIMIT_L1                = 0x7FFF         // Bits 0-14
	PKG_POWER_LIMIT_L1_ENABLED        = 0x8000         // Bit  15
	PKG_POWER_LIMIT_L2                = 0x7FFF00000000 // Bits 32-46
	PKG_POWER_LIMIT_L2_ENABLED        = 0x800000000000 // Bit  47
	UNCORE_RATIO_LIMIT_MAX            = 0x007F         // Bits 0-6
	UNCORE_RATIO_LIMIT_MIN            = 0x7F00         // Bits 8-14

	// Misc consts
	BASE_CLK_MHZ = 100
)

type powerInfo struct {
	thermalSpecPower   []uint64
	powerLimit1        []uint64
	powerLimit1Enabled []bool
	powerLimit2        []uint64
	powerLimit2Enabled []bool
}

type uncoreInfo struct {
	minRatio []uint64
	maxRatio []uint64
}

var logger = log.New(os.Stderr, "", log.LstdFlags)

func main() {
	// Get list of cpus
	cpus, err := getCpus()
	if err != nil {
		logger.Printf("FATAL: Failed to list CPUs: %s", err)
		os.Exit(1)
	}

	// Get list of physical packages
	packages, err := getCpuPackages(cpus)
	if err != nil {
		logger.Printf("FATAL: Failed to list physical CPU packages: %s", err)
		os.Exit(1)
	}

	// Get one cpu of each physical package
	packageCpus := []string{}
	for _, packageCpuList := range packages {
		packageCpus = append(packageCpus, packageCpuList[0])
	}

	// Detect TDP and power limits
	pInfo, err := getPowerInfo(packageCpus)
	if err != nil {
		logger.Printf("Failed to read CPU power info: %s", err)
	} else {
		if len(pInfo.thermalSpecPower) == 1 {
			fmt.Printf("tdp=%v\n", pInfo.thermalSpecPower[0])
		} else {
			logger.Printf("CPUs having different TDPs found, skipping 'tdp' label")
		}

		if len(pInfo.powerLimit1) == 1 && len(pInfo.powerLimit1Enabled) == 1 {
			if pInfo.powerLimit1Enabled[0] == true {
				fmt.Printf("power-limit-1=%v\n", pInfo.powerLimit1[0])
			} else {
				logger.Printf("CPU power limit 1 disabled")
			}
		} else {
			logger.Printf("CPUs with differing power limit 1 settings found, skipping 'power-limit-1' label")
		}

		if len(pInfo.powerLimit2) == 1 && len(pInfo.powerLimit2Enabled) == 1 {
			if pInfo.powerLimit2Enabled[0] == true {
				fmt.Printf("power-limit-2=%v\n", pInfo.powerLimit2[0])
			} else {
				logger.Printf("CPU power limit 2 disabled")
			}
		} else {
			logger.Printf("CPUs with differing power limit 2 settings found, skipping 'power-limit-2' label")
		}
	}

	// Detect C-state setting
	disabled, err := cstateDisabled(cpus)
	if err != nil {
		logger.Printf("Failed to detect C-state: %s", err)
	} else if disabled {
		fmt.Print("cstate-disabled\n")
	}

	// Detect uncore configuration
	uInfo, err := getUncoreInfo(packageCpus)
	if err != nil {
		logger.Printf("Failed to read uncore configuration info: %s", err)
	} else {
		if len(uInfo.minRatio) == 1 {
			fmt.Printf("uncore-min-frequency=%v\n", BASE_CLK_MHZ*uInfo.minRatio[0])
		} else {
			logger.Printf("Non-identical uncore min ratio settings detected, skipping 'uncore-min-frequency' label")
		}

		if len(uInfo.maxRatio) == 1 {
			fmt.Printf("uncore-max-frequency=%v\n", BASE_CLK_MHZ*uInfo.maxRatio[0])
		} else {
			logger.Printf("Non-identical uncore max ratio settings detected, skipping 'uncore-max-frequency' label")
		}
	}
}

// Get CPUs
func getCpus() ([]string, error) {
	const basePath = "/sys/bus/cpu/devices"
	cpus := []string{}

	ls, err := ioutil.ReadDir(basePath)
	if err != nil {
		return cpus, err
	}

	for _, cpu := range ls {
		// Strip 'cpu' prefix from the name of the cpu, i.e. only store the number
		cpus = append(cpus, cpu.Name()[3:])
	}

	return cpus, nil
}

// Get physical packages
func getCpuPackages(cpus []string) (map[string][]string, error) {
	const basePath = "/sys/bus/cpu/devices"
	cpuPackages := map[string][]string{}

	for _, cpu := range cpus {
		physPath := path.Join(basePath, "cpu"+cpu, "topology/physical_package_id")
		data, err := ioutil.ReadFile(physPath)
		if err != nil {
			return cpuPackages, fmt.Errorf("Failed to read CPU topology: %s", err)
		}
		physId := strings.TrimRight(string(data), "\n")
		cpuPackages[physId] = append(cpuPackages[physId], cpu)
	}

	return cpuPackages, nil
}

// Read one MSR
func readMsr(cpu string, msr int64) (uint64, error) {
	// Simply read from the first logical CPU
	f, err := os.Open(path.Join("/dev/cpu", cpu, "msr"))
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
		err = fmt.Errorf("short read on MSR 0x%x on CPU %v, %v of 8 bytes read", msr, cpu, n)
		return 0, err
	}

	// Convert byte slice to uint64
	return binary.LittleEndian.Uint64(buf), nil
}

// Read power info of multiple CPUs
func getPowerInfo(cpus []string) (powerInfo, error) {
	pInfo := powerInfo{}

	for _, cpu := range cpus {
		// Read power units
		unitsRaw, err := readMsr(cpu, MSR_RAPL_POWER_UNIT)
		if err != nil {
			return pInfo, err
		}
		unitsRaw = unitsRaw & RAPL_POWER_UNIT_POWER_UNITS
		units := 1 / float64((uint64(1) << unitsRaw))

		// Calculate thermal spec power
		powerRaw, err := readMsr(cpu, MSR_PKG_POWER_INFO)
		if err != nil {
			return pInfo, err
		}
		powerRaw = powerRaw & PKG_POWER_INFO_THERMAL_SPEC_POWER

		watts := uint64(float64(powerRaw) * units)
		if len(pInfo.thermalSpecPower) == 0 || pInfo.thermalSpecPower[0] != watts {
			// We're only interested in the uniqueness of values, thus stupidly
			// pushing values into the array
			pInfo.thermalSpecPower = append(pInfo.thermalSpecPower, watts)
		}

		// Read power limit MSR
		powerLimitRaw, err := readMsr(cpu, MSR_PKG_POWER_LIMIT)
		if err != nil {
			return pInfo, err
		}

		// Get power limit 1
		enabled := false
		if (powerLimitRaw & PKG_POWER_LIMIT_L1_ENABLED) > 0 {
			enabled = true
		}
		if len(pInfo.powerLimit1Enabled) == 0 || pInfo.powerLimit1Enabled[0] != enabled {
			// Dummy uniqueness trick
			pInfo.powerLimit1Enabled = append(pInfo.powerLimit1Enabled, enabled)
		}

		powerRaw = powerLimitRaw & PKG_POWER_LIMIT_L1
		watts = uint64(float64(powerRaw) * units)
		if len(pInfo.powerLimit1) == 0 || pInfo.powerLimit1[0] != watts {
			// Dummy uniqueness trick
			pInfo.powerLimit1 = append(pInfo.powerLimit1, watts)
		}

		// Get power limit 2
		enabled = false
		if (powerLimitRaw & PKG_POWER_LIMIT_L2_ENABLED) > 0 {
			enabled = true
		}
		if len(pInfo.powerLimit2Enabled) == 0 || pInfo.powerLimit2Enabled[0] != enabled {
			// Dummy uniqueness trick
			pInfo.powerLimit2Enabled = append(pInfo.powerLimit2Enabled, enabled)
		}

		powerRaw = (powerLimitRaw & PKG_POWER_LIMIT_L2) >> 32
		watts = uint64(float64(powerRaw) * units)
		if len(pInfo.powerLimit2) == 0 || pInfo.powerLimit2[0] != watts {
			// Dummy uniqueness trick
			pInfo.powerLimit2 = append(pInfo.powerLimit2, watts)
		}
	}

	return pInfo, nil
}

// Tell if all CPUs have C-state disabled
func cstateDisabled(cpus []string) (bool, error) {
	for _, cpu := range cpus {
		limit, err := readMsr(cpu, MSR_PKG_CST_CONFIG_CONTROL)
		if err != nil {
			return false, err
		}
		if limit&PKG_CST_CONFIG_CONTROL_LIMIT > 0 {
			return false, nil
		}
	}
	return true, nil
}

// Get uncore configuration information
func getUncoreInfo(cpus []string) (uncoreInfo, error) {
	uInfo := uncoreInfo{}

	for _, cpu := range cpus {
		ratioRaw, err := readMsr(cpu, MSR_UNCORE_RATIO_LIMIT)
		if err != nil {
			return uInfo, err
		}

		// Get min ratio
		ratio := (ratioRaw & UNCORE_RATIO_LIMIT_MIN) >> 8

		if len(uInfo.minRatio) == 0 || uInfo.minRatio[0] != ratio {
			// We're only interested if all values are identical, i.e. if len == 1
			uInfo.minRatio = append(uInfo.minRatio, ratio)
		}

		// Get max ratio
		ratio = ratioRaw & UNCORE_RATIO_LIMIT_MAX

		if len(uInfo.maxRatio) == 0 || uInfo.maxRatio[0] != ratio {
			// We're only interested if all values are identical
			uInfo.maxRatio = append(uInfo.maxRatio, ratio)
		}
	}
	return uInfo, nil
}
