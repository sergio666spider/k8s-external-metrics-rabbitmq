package main

import (
	"flag"
	"os"

	"github.com/golang/glog"
	basecmd "github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	fakeprov "github.com/sergio666spider/k8s-external-metrics-rabbitmq/pkg/provider"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/logs"
)

type RabbitmqAdapter struct {
	basecmd.AdapterBase

	Message string
}

func (a *RabbitmqAdapter) makeProviderOrDie() provider.MetricsProvider {
	client, err := a.DynamicClient()
	if err != nil {
		glog.Fatalf("unable to construct dynamic client: %v", err)
	}

	mapper, err := a.RESTMapper()
	if err != nil {
		glog.Fatalf("unable to construct discovery REST mapper: %v", err)
	}

	return fakeprov.NewRabbitmqProvider(client, mapper)
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	cmd := &RabbitmqAdapter{}
	cmd.Flags().StringVar(&cmd.Message, "msg", "starting adapter...", "startup message")
	cmd.Flags().AddGoFlagSet(flag.CommandLine) // make sure we get the glog flags
	cmd.Flags().Parse(os.Args)

	provider := cmd.makeProviderOrDie()
	cmd.WithCustomMetrics(provider)
	cmd.WithExternalMetrics(provider)

	glog.Infof(cmd.Message)
	if err := cmd.Run(wait.NeverStop); err != nil {
		glog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}
