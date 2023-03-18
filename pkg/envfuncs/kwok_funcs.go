/*
Copyright 2021 The Kubernetes Authors.

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

package envfuncs

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kwok"
)

type kwokContextKey string

// GetKwokClusterFromContext helps extract the kwok.Cluster object from the context.
// This can be used to setup and run tests of multi cluster kind.
func GetKwokClusterFromContext(ctx context.Context, clusterName string) (*kwok.Cluster, bool) {
	kwokCluster := ctx.Value(kwokContextKey(clusterName))
	if kwokCluster == nil {
		return nil, false
	}
	cluster, ok := kwokCluster.(*kwok.Cluster)
	return cluster, ok
}

// CreateKindCluster returns an env.Func that is used to
// create a kind cluster that is then injected in the context
// using the name as a key.
//
// NOTE: the returned function will update its env config with the
// kubeconfig file for the config client.
func CreateKwokCluster(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		k := kwok.NewCluster(clusterName)
		kubecfg, err := k.Create()
		if err != nil {
			return ctx, err
		}

		// update envconfig  with kubeconfig
		cfg.WithKubeconfigFile(kubecfg)

		// stall, wait for pods initializations
		if err := waitForKwokControlPlane(cfg.Client()); err != nil {
			return ctx, err
		}

		// store entire cluster value in ctx for future access using the cluster name
		return context.WithValue(ctx, kindContextKey(clusterName), k), nil
	}
}

// CreateKindClusterWithConfig returns an env.Func that is used to
// create a kind cluster that is then injected in the context
// using the name as a key.
//
// NOTE: the returned function will update its env config with the
// kubeconfig file for the config client.
func CreateKwokClusterWithConfig(clusterName, image, configFilePath string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		k := kwok.NewCluster(clusterName)
		kubecfg, err := k.CreateWithConfig(image, configFilePath)
		if err != nil {
			return ctx, err
		}

		// update envconfig  with kubeconfig
		cfg.WithKubeconfigFile(kubecfg)

		// stall, wait for pods initializations
		if err := waitForControlPlane(cfg.Client()); err != nil {
			return ctx, err
		}

		// store entire cluster value in ctx for future access using the cluster name
		return context.WithValue(ctx, kindContextKey(clusterName), k), nil
	}
}

func waitForKwokControlPlane(client klient.Client) error {
	r, err := resources.New(client.RESTConfig())
	if err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "component", Operator: metav1.LabelSelectorOpIn, Values: []string{"etcd", "kube-apiserver", "kube-controller-manager", "kube-scheduler"}},
			},
		},
	)
	if err != nil {
		return err
	}
	// a kind cluster with one control-plane node will have 4 pods running the core apiserver components
	err = wait.For(conditions.New(r).ResourceListN(&v1.PodList{}, 4, resources.WithLabelSelector(selector.String())))
	if err != nil {
		return err
	}
	selector, err = metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "k8s-app", Operator: metav1.LabelSelectorOpIn, Values: []string{"kindnet", "kube-dns", "kube-proxy"}},
			},
		},
	)
	if err != nil {
		return err
	}
	// a kind cluster with one control-plane node will have 4 k8s-app pods running networking components
	err = wait.For(conditions.New(r).ResourceListN(&v1.PodList{}, 4, resources.WithLabelSelector(selector.String())))
	if err != nil {
		return err
	}
	return nil
}

// DestroyKindCluster returns an EnvFunc that
// retrieves a previously saved kwok Cluster in the context (using the name), then deletes it.
//
// NOTE: this should be used in a Environment.Finish step.
func DestroyKwokCluster(name string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		clusterVal := ctx.Value(kindContextKey(name))
		if clusterVal == nil {
			return ctx, fmt.Errorf("destroy kwok cluster func: context cluster is nil")
		}

		cluster, ok := clusterVal.(*kwok.Cluster)
		if !ok {
			return ctx, fmt.Errorf("destroy kwok cluster func: unexpected type for cluster value")
		}

		if err := cluster.Destroy(); err != nil {
			return ctx, fmt.Errorf("destroy kind cluster: %w", err)
		}

		return ctx, nil
	}
}
