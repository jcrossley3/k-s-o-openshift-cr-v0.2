package webhook

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("webhook")

// AddToManagerFuncs is a list of functions to add all Webhooks to the Manager
var AddToManagerFuncs []func(manager.Manager) error

// AddToManager adds all Webhooks to the Manager
func AddToManager(m manager.Manager) error {

	if !runningOnOpenshift(m.GetConfig()) {
		log.Info("OpenShift not detected; no webhooks will be configured")
		return nil
	}

	for _, f := range AddToManagerFuncs {
		if err := f(m); err != nil {
			log.Error(err, "Unable to setup webhook")
			return err
		}
	}
	return nil
}

func runningOnOpenshift(cfg *rest.Config) bool {
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Can't create client")
		return false
	}
	gvk := schema.GroupVersionKind{"route.openshift.io", "v1", "route"}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	if err := c.List(context.TODO(), list); err != nil {
		if !meta.IsNoMatchError(err) {
			log.Error(err, "Unable to query for OpenShift Route")
		}
		return false
	}
	return true
}
