/*
Copyright © 2021 IBM Corporation

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

package kubeflux

import (
	"context"
	"google.golang.org/grpc"
	pb "sigs.k8s.io/scheduler-plugins/pkg/kubeflux/fluxcli-grpc"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/utils"
	"fmt"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/metrics"
	"sync"
	"time"
	
)

type KubeFlux struct {
	mutex          sync.Mutex
	handle         framework.Handle
	podNameToJobId map[string]uint64
}

var _ framework.PreFilterPlugin = &KubeFlux{}
var _ framework.FilterPlugin = &KubeFlux{}

// let's give it a name
const (
	Name = "KubeFlux"
)

func (kf *KubeFlux) Name() string {
	return Name
}

type fluxStateData struct {
	nodeName string
}

func (s *fluxStateData) Clone() framework.StateData {
	clone := &fluxStateData{
		nodeName: s.nodeName,
	}
	return clone
}

func (kf *KubeFlux) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	klog.Infof("Examining the pod")

	nodename, err := kf.askFlux(pod)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	if nodename == "NONE" {
		fmt.Println("Pod cannot be scheduled by KubeFlux, nodename ", nodename)
		return framework.NewStatus(framework.Unschedulable, "Pod cannot be scheduled by KubeFlux, nodename "+nodename)
	}

	fmt.Println("Node Selected: ", nodename)

	state.Write(framework.StateKey(pod.Name), &fluxStateData{nodeName: nodename})

	return framework.NewStatus(framework.Success, "")
}

func (kf *KubeFlux) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	fmt.Println("Filtering input node ", nodeInfo.Node().Name)
	if v, e := cycleState.Read(framework.StateKey(pod.Name)); e == nil {
		if value, ok := v.(*fluxStateData); ok && value.nodeName != nodeInfo.Node().Name {
			return framework.NewStatus(framework.Unschedulable, "pod is not permitted")
		} else {
			fmt.Println("Filter: node selected by Flux ", value.nodeName)
		}
	}

	return framework.NewStatus(framework.Success)
}

func (kf *KubeFlux) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (kf *KubeFlux) askFlux(pod *v1.Pod) (string, error) {

	// clean up previous match if a pod has already allocated previously
	kf.mutex.Lock()
	_, isPodAllocated := kf.podNameToJobId[pod.Name]
	kf.mutex.Unlock()

	if isPodAllocated {
		fmt.Println("Clean up previous allocation")
		kf.mutex.Lock()
		kf.cancelFluxJobForPod(pod.Name)
		kf.mutex.Unlock()
	}


	jobspec := utils.InspectPodInfo(pod)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())
	// or with sockets
	// conn, err := grpc.Dial(
	// 	sock,
	// 	grpc.WithInsecure(),
	// 	grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
	// 		return net.DialTimeout("unix", addr, timeout)
	// 	}))
	if err != nil {
		fmt.Println("[FluxClient] Error connecting to server: %v", err)
		return  "", err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()	

	request := &pb.MatchRequest{
		Ps: jobspec,
		Request: "allocate"}

	r, err2 := grpcclient.Match(context.Background(), request)
	if err2 != nil {
		fmt.Printf("[FluxClient] did not receive any match response: %v\n", err2)
		return "", err
	}

	fmt.Printf("[FluxClient] response nodeID %s\n", r.GetNodeID())
	fmt.Printf("[FluxClient] response podID %s\n", r.GetPodID())

	nodename := r.GetNodeID()
	jobid := uint64(r.GetJobID())

	kf.mutex.Lock()
	kf.podNameToJobId[pod.Name] = jobid
	fmt.Println("Check job set:")
	fmt.Println(kf.podNameToJobId)
	kf.mutex.Unlock()

	return nodename, nil
}

func (kf *KubeFlux) cancelFluxJobForPod(podName string) {
	jobid := kf.podNameToJobId[podName]

	fmt.Printf("Cancel flux job: %v for pod %s\n", jobid, podName)

	start := time.Now()
	//err := fluxcli.ReapiCliCancel(kf.fluxctx, int64(jobid), false)
	err := 1
	if err == 0 {
		delete(kf.podNameToJobId, podName)
	} else {
		fmt.Printf("Failed to delete pod %s from the podname-jobid map.\n", podName)
	}

	elapsed := metrics.SinceInSeconds(start)
	fmt.Println("Time elapsed (Cancel Job) :", elapsed)

	fmt.Printf("Job cancellation for pod %s result: %d\n", podName, err)
	fmt.Println("Check job set: after delete")
	fmt.Println(kf.podNameToJobId)
}

// initialize and return a new Flux Plugin
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("Error getting InClusterConfig")
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("Error getting ClientSet")
		return nil, err
	}

	factory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := factory.Core().V1().Pods().Informer()


	klog.Infof("KubeFlux starts")

	kf := &KubeFlux{handle: handle, podNameToJobId: make(map[string]uint64)}

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: kf.updatePod,
	})

	stopPodInformer := make(chan struct{})
	go podInformer.Run(stopPodInformer)

	return kf, nil
}

// EventHandlers
func (kf *KubeFlux) updatePod(oldObj, newObj interface{}) {
	fmt.Println("updatePod event handler")
	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)
	fmt.Println(oldPod)
	fmt.Println(newPod)
	fmt.Println(oldPod.Name, oldPod.Status)
	fmt.Println(newPod.Name, newPod.Status)

	switch newPod.Status.Phase {
	case v1.PodPending:
		// in this state we don't know if a pod is going to be running, thus we don't need to update job map
	case v1.PodRunning:
		// if a pod is start running, we can add it state to the delta graph if it is scheduled by other scheduler
	case v1.PodSucceeded:
		fmt.Printf("Pod %s succeeded, kubeflux needs to free the resources\n", newPod.Name)

		kf.mutex.Lock()
		defer kf.mutex.Unlock()

		if _, ok := kf.podNameToJobId[newPod.Name]; ok {
			kf.cancelFluxJobForPod(newPod.Name)
		} else {
			fmt.Printf("Succeeded pod %s/%s doesn't have flux jobid\n", newPod.Namespace, newPod.Name)
		}
	case v1.PodFailed:
		// a corner case need to be tested, the pod exit code is not 0, can be created with segmentation fault pi test
		fmt.Printf("Pod %s failed, kubeflux needs to free the resources\n", newPod.Name)

		kf.mutex.Lock()
		defer kf.mutex.Unlock()

		if _, ok := kf.podNameToJobId[newPod.Name]; ok {
			kf.cancelFluxJobForPod(newPod.Name)
		} else {
			fmt.Printf("Failed pod %s/%s doesn't have flux jobid\n", newPod.Namespace, newPod.Name)
		}
	case v1.PodUnknown:
		// don't know how to deal with it as it's unknown phase
	default:
		// shouldn't enter this branch
	}

}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, overhead, fluxerr)
}
