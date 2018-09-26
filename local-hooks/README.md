# CPU Feature Detector

This directory contains a tool for detecting certain CPU related configuration
features, designed to be run as a custom feature source detector hook of Node
Feature Discovery in a Kubernetes cluster.


## Overview

The `cpu` tool will detect CPU features and print the discovered configuration
in `stdout`, one feature per line. Labels represent system-wide settings, and
thus, all CPUs must have identical configuration of certain feature for the
corresponding label to be advertised. That is, if, for example, if all CPUs in
a system have the same value configured for RAPL power limit 1, but different
values for power limit 2, only 'power-limit-1' label gets published.

The tool currently detects the following features:

| Feature      | Labels                 | Description
| :----------- | :-------------------:  | :-----------
| TDP          | `tdp`                  | Nominal TDP of the CPU (Watts)
| Power limits | `power-limit-1`        | RAPL power limits configured for the CPU (Watts)
| .            | `power-limit-2` |
| C-state      | `cstate-disabled`      | C-state configuration, tells if all C-states have been disabled
| Uncore freq  | `uncore-min-frequency` | Uncore frequency configuration, min/max range of possible uncore frequencies.
| .            | `uncore-max-frequency`

NFD will prefix the label with `node.alpha.kubernetes-incubator.io/nfd-cpu-`, so
that the full node label visible in Kubernetes will be:
```
node.alpha.kubernetes-incubator.io/nfd-local-cpu-<CPU_LABEL_NAME> = <CPU_LABEL_VALUE>
```


## Building and Running

To buid the tool simply run
```
make
```

The binary needs super user privileges in order to read model specific registers
(MSR). To try it out, run it with sudo:
```
$ sudo ./cpu
tdp=145
power-limit-1=145
power-limit-2=181
cstate-disabled
uncore-min-frequency=1200
uncore-max-frequency=2800
```


## Deployment in NFD

The `cpu` hook is automatically installed and run if you build and run a custom
NFD image from this Git branch.

However, future versions of upstream NFD probably have support for the hook
mechanism. In this case, you can use officially released NFD image and just
mount the hook inside the container.

### Mounting CPU Hook inside NFD

We present two alternative ways to accomplish this here.

#### Mount From Network Volume

Here, you build the `cpu` binary locally, make it available from a network
filesystem, and, mount it inside the NFD container.

An example when using NFS, after running `make` and copying `cpu` binary onto
your NFS share, something like the snippet below would be specified in your NFD
Pod spec:
```
  volumes:
  - name: nfd-hooks
    nfs:
      server: <YOUR_NFS_SERVER_ADDR>
      # An example path that would contain cpu (and possibly other hooks)
      path: "/nfd-hooks"
      readOnly: true
...
  containers:
  - volumeMounts:
    - name: nfd-hooks
      mountPath: "/etc/kubernetes/node-feature-discovery/source.d"
      readOnly: true
...
```

#### Use Init Container

Alternatively, you could use an init container for installing the `cpu` binary
inside the NFD container. In this case you do not need to rely on remote file
systems. We provide a Dockerfile for building this init container.

Steps to build and deploy the `cpu` hook using an init container:
1. Build the init container: `docker build -t <INIT_CONTAINER_DOCKER_TAG> local-hooks/`
1. Push the init container into your registry: `docker push <INIT_CONTAINER_DOCKER_TAG>
1. Deploy NFD with you init container installing the `cpu` hook. The following
snippet containing the relevant parts:

```
  initContainers:
  - image: <INIT_CONTAINER_DOCKER_TAG>
    imagePullPolicy: Always
    name: install-nfd-hooks
    command: ["cp", "/cpu", "/mnt/nfd-hooks"]
    volumeMounts:
    - name: nfd-hooks
      mountPath: "/mnt/nfd-hooks"
  containers:
  - volumeMounts:
    - name: nfd-hooks
      mountPath: "/etc/kubernetes/node-feature-discovery/source.d"
      readOnly: true
...
  volumes:
  - name: nfd-hooks
    emptyDir: {}
...

```

### Additional NFD Pod Configuration

In addition to getting the `cpu` hook installed in your NFD container, there
are two other Pod configuration settings that you need in order for the hook to
correctly operate:
1. `/dev/cpu` needs to be mounted inside the container to expose `msr` device nodes
1. The container needs to run in privileged mode in order for the tool to be permitted read MSRs

The snippet below shows an example of the additional configuration required:
```
  containers:
  - securityContext:
    # Privileged mode required to read MSRs
      privileged: true
    volumeMounts:
    # /dev/cpu needs to be mounted in order to expose msr device nodes
    - name: host-devcpu
      mountPath: "/dev/cpu"
...
  volumes:
  - name: host-devcpu
    hostPath:
      path: "/dev/cpu"
...
```

There is a spec template (`node-feature-discovery-daemonset-initcontainer.yaml`)
representing a complete example of deploying NFD with an init container handling
the installation of the `cpu` hook inside NFD.
