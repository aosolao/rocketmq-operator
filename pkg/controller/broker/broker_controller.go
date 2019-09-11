/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package broker

import (
	"context"
	"strconv"
	"time"

	rocketmqv1alpha1 "github.com/operator-sdk-samples/rocketmq-operator/pkg/apis/rocketmq/v1alpha1"
	cons "github.com/operator-sdk-samples/rocketmq-operator/pkg/constants"
	"github.com/operator-sdk-samples/rocketmq-operator/pkg/share"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_broker")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Broker Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBroker{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("broker-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Broker
	err = c.Watch(&source.Kind{Type: &rocketmqv1alpha1.Broker{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Broker
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &rocketmqv1alpha1.Broker{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileBroker implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileBroker{}

// ReconcileBroker reconciles a Broker object
type ReconcileBroker struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Broker object and makes changes based on the state read
// and what is in the Broker.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileBroker) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Broker.")

	// Fetch the Broker instance
	broker := &rocketmqv1alpha1.Broker{}
	err := r.client.Get(context.TODO(), request.NamespacedName, broker)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Broker resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Broker.")
		return reconcile.Result{}, err
	}

	share.GroupNum = int(broker.Spec.Size)
	share.BrokerClusterName = broker.Name
	replicaPerGroup := broker.Spec.ReplicaPerGroup
	reqLogger.Info("brokerGroupNum=" + strconv.Itoa(share.GroupNum) + ", replicaPerGroup=" + strconv.Itoa(replicaPerGroup))

	for brokerGroupIndex := 0; brokerGroupIndex < share.GroupNum; brokerGroupIndex++ {
		brokerName := getBrokerName(broker, brokerGroupIndex)
		reqLogger.Info("Check Broker cluster " + strconv.Itoa(brokerGroupIndex+1) + "/" + strconv.Itoa(share.GroupNum))
		dep := r.statefulSetForMasterBroker(broker, brokerGroupIndex)
		// Check if the statefulSet already exists, if not create a new one
		found := &appsv1.StatefulSet{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)

		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Creating a new Master Broker StatefulSet.", "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
			err = r.client.Create(context.TODO(), dep)
			if err != nil {
				reqLogger.Error(err, "Failed to create new StatefulSet of "+cons.BrokerClusterPrefix+strconv.Itoa(brokerGroupIndex), "StatefulSet.Namespace", dep.Namespace, "StatefulSet.Name", dep.Name)
			}
		} else if err != nil {
			reqLogger.Error(err, "Failed to get broker master StatefulSet.")
		}

		for replicaIndex := 1; replicaIndex <= replicaPerGroup; replicaIndex++ {
			reqLogger.Info("Check Replica Broker of cluster-" + strconv.Itoa(brokerGroupIndex) + " " + strconv.Itoa(replicaIndex) + "/" + strconv.Itoa(replicaPerGroup))
			replicaDep := r.statefulSetForReplicaBroker(broker, brokerGroupIndex, replicaIndex)
			err = r.client.Get(context.TODO(), types.NamespacedName{Name: replicaDep.Name, Namespace: replicaDep.Namespace}, found)
			if err != nil && errors.IsNotFound(err) {
				reqLogger.Info("Creating a new Replica Broker StatefulSet.", "StatefulSet.Namespace", replicaDep.Namespace, "StatefulSet.Name", replicaDep.Name)
				err = r.client.Create(context.TODO(), replicaDep)
				if err != nil {
					reqLogger.Error(err, "Failed to create new StatefulSet of broker-"+strconv.Itoa(brokerGroupIndex)+"-replica-"+strconv.Itoa(replicaIndex), "StatefulSet.Namespace", replicaDep.Namespace, "StatefulSet.Name", replicaDep.Name)
				}
			} else if err != nil {
				reqLogger.Error(err, "Failed to get broker replica StatefulSet.")
			}
		}

		if broker.Spec.AllowRestart {
			// The following code will restart all brokers to update NAMESRV_ADDR env
			if share.IsNameServersStrUpdated {
				// update master broker
				reqLogger.Info("Update Master Broker NAMESRV_ADDR of " + brokerName)
				err = r.client.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, found)
				if err != nil {
					reqLogger.Error(err, "Failed to get broker master StatefulSet of " + brokerName)
				} else {
					found.Spec.Template.Spec.Containers[0].Env[0].Value = share.NameServersStr
					err = r.client.Update(context.TODO(), found)
					if err != nil {
						reqLogger.Error(err, "Failed to update NAMESRV_ADDR of master broker " + brokerName, "StatefulSet.Namespace", found.Namespace, "StatefulSet.Name", found.Name)
					} else {
						reqLogger.Info("Successfully updated NAMESRV_ADDR of master broker " + brokerName, "StatefulSet.Namespace", found.Namespace, "StatefulSet.Name", found.Name)
					}
				}
				// update replicas brokers
				for replicaIndex := 1; replicaIndex <= replicaPerGroup; replicaIndex++ {
					reqLogger.Info("Update Replica Broker NAMESRV_ADDR of " + brokerName + " " + strconv.Itoa(replicaIndex) + "/" + strconv.Itoa(replicaPerGroup))
					replicaDep := r.statefulSetForReplicaBroker(broker, brokerGroupIndex, replicaIndex)
					replicaFound := &appsv1.StatefulSet{}
					err = r.client.Get(context.TODO(), types.NamespacedName{Name: replicaDep.Name, Namespace: replicaDep.Namespace}, replicaFound)
					if err != nil {
						reqLogger.Error(err, "Failed to get broker replica StatefulSet of " + brokerName)
					} else {
						replicaFound.Spec.Template.Spec.Containers[0].Env[0].Value = share.NameServersStr
						err = r.client.Update(context.TODO(), replicaFound)
						if err != nil {
							reqLogger.Error(err, "Failed to update NAMESRV_ADDR of "+strconv.Itoa(brokerGroupIndex)+"-replica-"+strconv.Itoa(replicaIndex), "StatefulSet.Namespace", replicaFound.Namespace, "StatefulSet.Name", replicaFound.Name)
						} else {
							reqLogger.Info("Successfully updated NAMESRV_ADDR of "+strconv.Itoa(brokerGroupIndex)+"-replica-"+strconv.Itoa(replicaIndex), "StatefulSet.Namespace", replicaFound.Namespace, "StatefulSet.Name", replicaFound.Name)
						}
					}
				}
			}
		}
	}
	share.IsNameServersStrUpdated = false

	// Ensure the statefulSet size is the same as the spec
	//size := broker.Spec.Size
	//if *found.Spec.Replicas != size {
	//	found.Spec.Replicas = &size
	//	err = r.client.Update(context.TODO(), found)
	//	if err != nil {
	//		reqLogger.Error(err, "Failed to update StatefulSet.", "StatefulSet.Namespace", found.Namespace, "StatefulSet.Name", found.Name)
	//		return reconcile.Result{}, err
	//	}
	//	// Spec updated - return and requeue
	//	return reconcile.Result{Requeue: true}, nil
	//}

	// Update the Broker status with the pod names
	// List the pods for this broker's statefulSet

	//podList := &corev1.PodList{}
	//labelSelector := labels.SelectorFromSet(labelsForBroker(broker.Name))
	//listOps := &client.ListOptions{
	//	Namespace:     broker.Namespace,
	//	LabelSelector: labelSelector,
	//}
	//err = r.client.List(context.TODO(), listOps, podList)
	//if err != nil {
	//	reqLogger.Error(err, "Failed to list pods.", "Broker.Namespace", broker.Namespace, "Broker.Name", broker.Name)
	//	return reconcile.Result{}, err
	//}
	//podNames := getPodNames(podList.Items)
	//
	//// Update status.Nodes if needed
	//if !reflect.DeepEqual(podNames, broker.Status.Nodes) {
	//	broker.Status.Nodes = podNames
	//	err := r.client.Status().Update(context.TODO(), broker)
	//	if err != nil {
	//		reqLogger.Error(err, "Failed to update Broker status.")
	//		return reconcile.Result{}, err
	//	}
	//}

	return reconcile.Result{true, time.Duration(3) * time.Second}, nil
}


func getBrokerName(broker *rocketmqv1alpha1.Broker, brokerGroupIndex int) string {
	return broker.Name + "-" + strconv.Itoa(brokerGroupIndex)
}

// statefulSetForBroker returns a master broker StatefulSet object
func (r *ReconcileBroker) statefulSetForMasterBroker(m *rocketmqv1alpha1.Broker, brokerGroupIndex int) *appsv1.StatefulSet {
	ls := labelsForBroker(m.Name)
	var a int32 = 1
	var c = &a
	dep := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-" + strconv.Itoa(brokerGroupIndex) + "-master",
			Namespace: m.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: c,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           m.Spec.BrokerImage,
						Name:            cons.MasterBrokerContainerNamePrefix + strconv.Itoa(brokerGroupIndex),
						ImagePullPolicy: m.Spec.ImagePullPolicy,
						Env: []corev1.EnvVar{{
							Name:  cons.EnvNameServiceAddress,
							Value: m.Spec.NameServers,
						}, {
							Name:  cons.EnvReplicationMode,
							Value: m.Spec.ReplicationMode,
						}, {
							Name:  cons.EnvBrokerId,
							Value: "0",
						}, {
							Name:  cons.EnvBrokerClusterName,
							Value: m.Name,
						}, {
							Name:  cons.EnvBrokerName,
							Value: m.Name + "-" + strconv.Itoa(brokerGroupIndex),
						}},
						Ports: []corev1.ContainerPort{{
							ContainerPort: cons.BrokerVipContainerPort,
							Name:          cons.BrokerVipContainerPortName,
						}, {
							ContainerPort: cons.BrokerMainContainerPort,
							Name:          cons.BrokerMainContainerPortName,
						}, {
							ContainerPort: cons.BrokerHighAvailabilityContainerPort,
							Name:          cons.BrokerHighAvailabilityContainerPortName,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: cons.LogMountPath,
							Name: m.Spec.VolumeClaimTemplates[0].Name,
							SubPath: cons.LogSubPathName,
						},{
							MountPath: cons.StoreMountPath,
							Name: m.Spec.VolumeClaimTemplates[0].Name,
							SubPath: cons.StoreSubPathName,
						}},
					}},
				},
			},
			VolumeClaimTemplates: m.Spec.VolumeClaimTemplates,
		},
	}
	// Set Broker instance as the owner and controller
	controllerutil.SetControllerReference(m, dep, r.scheme)

	return dep

}

// statefulSetForBroker returns a replica broker StatefulSet object
func (r *ReconcileBroker) statefulSetForReplicaBroker(m *rocketmqv1alpha1.Broker, brokerGroupIndex int, replicaIndex int) *appsv1.StatefulSet {
	ls := labelsForBroker(m.Name)
	var a int32 = 1
	var c = &a
	dep := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-" + strconv.Itoa(brokerGroupIndex) + "-replica-" + strconv.Itoa(replicaIndex),
			Namespace: m.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: c,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           m.Spec.BrokerImage,
						Name:            cons.ReplicaBrokerContainerNamePrefix + strconv.Itoa(brokerGroupIndex),
						ImagePullPolicy: m.Spec.ImagePullPolicy,
						Env: []corev1.EnvVar{{
							Name:  cons.EnvNameServiceAddress,
							Value: m.Spec.NameServers,
						}, {
							Name:  cons.EnvReplicationMode,
							Value: m.Spec.ReplicationMode,
						}, {
							Name:  cons.EnvBrokerId,
							Value: strconv.Itoa(replicaIndex),
						}, {
							Name:  cons.EnvBrokerClusterName,
							Value: m.Name,
						}, {
							Name:  cons.EnvBrokerName,
							Value: m.Name + "-" + strconv.Itoa(brokerGroupIndex),
						}},
						Ports: []corev1.ContainerPort{{
							ContainerPort: cons.BrokerVipContainerPort,
							Name:          cons.BrokerVipContainerPortName,
						}, {
							ContainerPort: cons.BrokerMainContainerPort,
							Name:          cons.BrokerMainContainerPortName,
						}, {
							ContainerPort: cons.BrokerHighAvailabilityContainerPort,
							Name:          cons.BrokerHighAvailabilityContainerPortName,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							MountPath: cons.LogMountPath,
							Name: m.Spec.VolumeClaimTemplates[0].Name,
							SubPath: cons.LogSubPathName,
						},{
							MountPath: cons.StoreMountPath,
							Name: m.Spec.VolumeClaimTemplates[0].Name,
							SubPath: cons.StoreSubPathName,
						}},
					}},
				},
			},
			VolumeClaimTemplates: m.Spec.VolumeClaimTemplates,
		},
	}
	// Set Broker instance as the owner and controller
	controllerutil.SetControllerReference(m, dep, r.scheme)

	return dep

}

// labelsForBroker returns the labels for selecting the resources
// belonging to the given broker CR name.
func labelsForBroker(name string) map[string]string {
	return map[string]string{"app": "broker", "broker_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}