package namespace

/*
Copyright 2019 - 2020 Crunchy Data Solutions, Inc.
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

import (
	"context"

	"github.com/crunchydata/postgres-operator/controller/pod"

	"github.com/crunchydata/postgres-operator/config"
	"github.com/crunchydata/postgres-operator/controller/job"
	"github.com/crunchydata/postgres-operator/controller/pgcluster"
	"github.com/crunchydata/postgres-operator/controller/pgpolicy"
	"github.com/crunchydata/postgres-operator/controller/pgreplica"
	"github.com/crunchydata/postgres-operator/controller/pgtask"
	"github.com/crunchydata/postgres-operator/operator"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Controller holds the connections for the controller
type Controller struct {
	NamespaceClient        *rest.RESTClient
	NamespaceClientset     *kubernetes.Clientset
	Ctx                    context.Context
	PodController       *pod.Controller
	JobController       *job.Controller
	PgpolicyController  *pgpolicy.Controller
	PgreplicaController *pgreplica.Controller
	PgclusterController *pgcluster.Controller
	PgtaskController    *pgtask.Controller
}

// Run starts a namespace resource controller
func (c *Controller) Run() error {

	err := c.watchNamespaces(c.Ctx)
	if err != nil {
		log.Errorf("Failed to register watch for namespace resource: %v", err)
		return err
	}

	<-c.Ctx.Done()
	return c.Ctx.Err()
}

// watchNamespaces is the event loop for namespace resources
func (c *Controller) watchNamespaces(ctx context.Context) error {
	log.Info("starting namespace controller")

	//watch all namespaces
	ns := ""

	source := cache.NewListWatchFromClient(
		c.NamespaceClientset.CoreV1().RESTClient(),
		"namespaces",
		ns,
		fields.Everything())

	_, controller := cache.NewInformer(
		source,

		// The object type.
		&v1.Namespace{},

		// resyncPeriod
		// Every resyncPeriod, all resources in the cache will retrigger events.
		// Set to 0 to disable the resync.
		0,

		// Your custom resource event handlers.
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		})

	go controller.Run(ctx.Done())

	return nil
}

func (c *Controller) onAdd(obj interface{}) {
	newNs := obj.(*v1.Namespace)

	log.Debugf("[namespace Controller] OnAdd ns=%s", newNs.ObjectMeta.SelfLink)
	labels := newNs.GetObjectMeta().GetLabels()
	if labels[config.LABEL_VENDOR] != config.LABEL_CRUNCHY || labels[config.LABEL_PGO_INSTALLATION_NAME] != operator.InstallationName {
		log.Debugf("namespace Controller: onAdd skipping namespace that is not crunchydata or not belonging to this Operator installation %s", newNs.ObjectMeta.SelfLink)
		return
	} else {
		log.Debugf("namespace Controller: onAdd crunchy namespace %s created", newNs.ObjectMeta.SelfLink)
		c.PodController.SetupWatch(newNs.Name)
		c.JobController.SetupWatch(newNs.Name)
		c.PgpolicyController.SetupWatch(newNs.Name)
		c.PgreplicaController.SetupWatch(newNs.Name)
		c.PgclusterController.SetupWatch(newNs.Name)
		c.PgtaskController.SetupWatch(newNs.Name)
	}

}

// onUpdate is called when a pgcluster is updated
func (c *Controller) onUpdate(oldObj, newObj interface{}) {
	//oldNs := oldObj.(*v1.Namespace)
	newNs := newObj.(*v1.Namespace)
	log.Debugf("[namespace Controller] onUpdate ns=%s", newNs.ObjectMeta.SelfLink)

	labels := newNs.GetObjectMeta().GetLabels()
	if labels[config.LABEL_VENDOR] != config.LABEL_CRUNCHY || labels[config.LABEL_PGO_INSTALLATION_NAME] != operator.InstallationName {
		log.Debugf("namespace Controller: onUpdate skipping namespace that is not crunchydata %s", newNs.ObjectMeta.SelfLink)
		return
	} else {
		log.Debugf("namespace Controller: onUpdate crunchy namespace updated %s", newNs.ObjectMeta.SelfLink)
		c.PodController.SetupWatch(newNs.Name)
		c.JobController.SetupWatch(newNs.Name)
		c.PgpolicyController.SetupWatch(newNs.Name)
		c.PgreplicaController.SetupWatch(newNs.Name)
		c.PgclusterController.SetupWatch(newNs.Name)
		c.PgtaskController.SetupWatch(newNs.Name)
	}

}

func (c *Controller) onDelete(obj interface{}) {
	ns := obj.(*v1.Namespace)

	log.Debugf("[namespace Controller] onDelete ns=%s", ns.ObjectMeta.SelfLink)
	labels := ns.GetObjectMeta().GetLabels()
	if labels[config.LABEL_VENDOR] != config.LABEL_CRUNCHY {
		log.Debugf("namespace Controller: onDelete skipping namespace that is not crunchydata %s", ns.ObjectMeta.SelfLink)
		return
	} else {
		log.Debugf("namespace Controller: onDelete crunchy operator namespace %s is deleted", ns.ObjectMeta.SelfLink)
	}

}
