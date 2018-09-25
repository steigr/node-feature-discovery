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
	MSR_RAPL_POWER_UNIT = 0x606
	MSR_PKG_POWER_INFO  = 0x614

	// Bit masks for the MSRs
	RAPL_POWER_UNIT_POWER_UNITS       = 0x0007 // Bits 0-2
	PKG_POWER_INFO_THERMAL_SPEC_POWER = 0x7FFF // Bits 0-14
)

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

	// Detect TDP
	tdps, err := thermalSpecPower(packageCpus)
	if err != nil {
		logger.Printf("Failed to detect TDP: %s", err)
	} else {
		if len(tdps) > 1 {
			fmt.Printf("CPUs having different TDPs found, skipping 'tdp' label")
		} else {
			for tdp := range tdps {
				fmt.Printf("tdp=%v\n", tdp)
			}
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

// Read TDP (Thermal Spec Power) of multiple CPUs
func thermalSpecPower(cpus []string) (map[uint64][]string, error) {
	tdp := map[uint64][]string{}

	for _, cpu := range cpus {
		// Read power units
		unitsRaw, err := readMsr(cpu, MSR_RAPL_POWER_UNIT)
		if err != nil {
			return nil, err
		}
		unitsRaw = unitsRaw & RAPL_POWER_UNIT_POWER_UNITS
		units := 1 / float64((uint64(1) << unitsRaw))

		// Calculate thermal spec power
		powerRaw, err := readMsr(cpu, MSR_PKG_POWER_INFO)
		if err != nil {
			return nil, err
		}
		powerRaw = powerRaw & PKG_POWER_INFO_THERMAL_SPEC_POWER

		watts := uint64(float64(powerRaw) * units)
		tdp[watts] = append(tdp[watts], cpu)
	}

	return tdp, nil
}
