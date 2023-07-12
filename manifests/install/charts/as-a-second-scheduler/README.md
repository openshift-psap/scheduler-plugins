# Scheduler-plugins as a second scheduler in cluster

## Installation

Quick start instructions for the setup and configuration of as-a-second-scheduler using Helm.

### Prerequisites

- [Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Installing the chart

#### Fluence specific values

In `values.yaml` it is possible to customize the container image, already defaulted to the latest release, and the allocation policy
used by the scheduler.
Most common options are:

- `lonode`: choose the nodes with lower ID first. Can be compared to packing
- `low`: choose cores with lowest IDs from multiple nodes. Can be compared to spread process-to-resource placement


#### Install chart using Helm v3.0+

```bash
$ git clone https://github.com/openshift-psap/scheduler-plugins.git -b fluence
$ cd scheduler-plugins/manifests/install/charts
$ helm install scheduler-plugins as-a-second-scheduler/
```

#### Verify that scheduler and plugin-controller pod are running properly.

```bash
$ kubectl get deploy -n scheduler-plugins
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
scheduler-plugins-controller   1/1     1            1           7s
scheduler-plugins-scheduler    1/1     1            1           7s
```

### Configuration

The following table lists the configurable parameters of the as-a-second-scheduler chart and their default values.

| Parameter                               | Description                   | Default                                                                                         |
| --------------------------------------- |-------------------------------|-------------------------------------------------------------------------------------------------|
| `scheduler.name`                        | Scheduler name                | `scheduler-plugins-scheduler`                                                                   |
| `scheduler.image`                       | Scheduler image               | `k8s.gcr.io/scheduler-plugins/kube-scheduler:v0.23.10`                                          |
| `scheduler.namespace`                   | Scheduler namespace           | `scheduler-plugins`                                                                             |
| `scheduler.replicaCount`                | Scheduler replicaCount        | `1`                                                                                             |
| `controller.name`                       | Controller name               | `scheduler-plugins-controller`                                                                  |
| `controller.image`                      | Controller image              | `k8s.gcr.io/scheduler-plugins/controller:v0.23.10`                                              |
| `controller.namespace`                  | Controller namespace          | `scheduler-plugins`                                                                             |
| `controller.replicaCount`               | Controller replicaCount       | `1`                                                                                             |
| `plugins.enabled`                       | Plugins enabled by default    | `["Coscheduling","CapacityScheduling","NodeResourceTopologyMatch", "NodeResourcesAllocatable"]` |
| `plugins.enabled`                       | Plugins disabled by default   | `["PrioritySort"]`                                                                              |

