/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"

	"github.com/samutayuga/app-operator/api/v1alpha1"
	solutionfeatureaerov1alpha1 "github.com/samutayuga/app-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// BackendReconciler reconciles a Backend object
type BackendReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=solution.feature.aero.sample.com,resources=backends,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=solution.feature.aero.sample.com,resources=backends/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=solution.feature.aero.sample.com,resources=backends/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Backend object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *BackendReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	controllerLogger := ctrllog.FromContext(ctx)
	//get the backend cr
	backendCr := &v1alpha1.Backend{}
	err := r.Get(ctx, req.NamespacedName, backendCr)
	// your logic here
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			controllerLogger.Info("Backend resources not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
	}
	//check if deployment exist
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: backendCr.Name, Namespace: backendCr.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		//need to deploy
		dep := r.Deploy(backendCr)
		controllerLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			controllerLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		//successfully create deployment

	} else if err != nil {
		controllerLogger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}
	//check if service exists
	foundSvc := &v1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: backendCr.Name, Namespace: backendCr.Namespace}, foundSvc)
	if err != nil {
		if errors.IsNotFound(err) {
			//ctrl.SetControllerReference(backendCr, foundSvc, r.Scheme)
			svr := r.DeployService(backendCr)
			controllerLogger.Info("Creating a new Service", "Service.Namespace", svr.Namespace, "Service.Name", svr.Name)
			err = r.Client.Create(ctx, svr)
			if err != nil {
				controllerLogger.Error(err, "Failed to create new Service", "Service.Namespace", svr.Namespace, "Service.Name", svr.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}
	//svr := r.DeployService(backendCr)

	//err = r.Create(ctx, svr)

	//compare actual deployment and its desired state
	apps := backendCr.Spec.BackendApps
	for _, app := range apps {
		anImage := fmt.Sprintf("%s:%s", app.Image, app.Version)
		conts := found.Spec.Template.Spec.Containers
		for _, cont := range conts {
			if cont.Image == anImage {
				return ctrl.Result{}, nil
			} else {
				dep := r.Deploy(backendCr)
				controllerLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
				err = r.Create(ctx, dep)
				if err != nil {
					controllerLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
					return ctrl.Result{}, err
				}

				//successfully create deployment
				return ctrl.Result{Requeue: true}, nil

			}
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackendReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&solutionfeatureaerov1alpha1.Backend{}).
		Complete(r)
}

func buildContainers(apps []v1alpha1.BackendApp) []v1.Container {
	contList := make([]v1.Container, 0)
	for _, v := range apps {
		aCont := v1.Container{
			Name:  v.Name,
			Image: fmt.Sprintf("%s:%s", v.Image, v.Version),
			Ports: []v1.ContainerPort{{Name: v.Name, ContainerPort: int32(v.Port)}}}
		contList = append(contList, aCont)
	}
	return contList
}
func (r *BackendReconciler) Deploy(backend *solutionfeatureaerov1alpha1.Backend) *appsv1.Deployment {
	replica := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backend.Name,
			Namespace: backend.Namespace,
			Labels:    labels(backend.GetObjectMeta().GetName()),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replica,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels(backend.Name),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backend.Name,
					Namespace: backend.Namespace,
					Labels:    labels(backend.Name),
				},
				Spec: v1.PodSpec{
					Containers: buildContainers(backend.Spec.BackendApps)},
			},
		},
	}
	ctrl.SetControllerReference(backend, dep, r.Scheme)
	return dep
}
func (r *BackendReconciler) DeployService(backend *solutionfeatureaerov1alpha1.Backend) *v1.Service {
	srv := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backend.Name,
			Namespace: backend.Namespace,
			Labels:    labels(backend.Name),
		},
		Spec: v1.ServiceSpec{
			Selector: labels(backend.Name),
			Ports:    portList(backend.Spec.BackendApps),
		},
	}
	ctrl.SetControllerReference(backend, srv, r.Scheme)
	return srv
}
func portList(apps []v1alpha1.BackendApp) []v1.ServicePort {
	contList := make([]v1.ServicePort, 0)
	for _, v := range apps {
		aCont := v1.ServicePort{
			Name:       v.Name,
			Port:       int32(v.Port),
			Protocol:   v1.ProtocolTCP,
			TargetPort: intstr.FromString(v.Name),
		}
		contList = append(contList, aCont)
	}
	return contList
}
func labels(name string) map[string]string {
	return map[string]string{"app": name}
}
