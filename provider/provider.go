/*
Copyright 2017 The Kubernetes Authors.

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

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider/helpers"
)

const (
	userid            = "test"
	password          = "test"
	queue_metric_name = "queue-messages"
)

type IP_Origin struct {
	Origin string
}

type Queue_Metrics struct {
	Name     string
	Messages int64
}

type externalMetric struct {
	info   provider.ExternalMetricInfo
	labels map[string]string
	value  external_metrics.ExternalMetricValue
}

type testingProvider struct {
	client dynamic.Interface
	mapper apimeta.RESTMapper

	values          map[provider.CustomMetricInfo]int64
	externalMetrics []externalMetric
}

var (
	testingMetrics = []externalMetric{
		{
			info: provider.ExternalMetricInfo{
				Metric: "my-external-metric",
			},
			labels: map[string]string{"foo": "bar"},
			value: external_metrics.ExternalMetricValue{
				MetricName: "my-external-metric",
				MetricLabels: map[string]string{
					"foo": "bar",
				},
				Value: *resource.NewQuantity(42, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: "my-external-metric",
			},
			labels: map[string]string{"foo": "baz"},
			value: external_metrics.ExternalMetricValue{
				MetricName: "my-external-metric",
				MetricLabels: map[string]string{
					"foo": "baz",
				},
				Value: *resource.NewQuantity(43, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: "other-external-metric",
			},
			labels: map[string]string{},
			value: external_metrics.ExternalMetricValue{
				MetricName:   "other-external-metric",
				MetricLabels: map[string]string{},
				Value:        *resource.NewQuantity(44, resource.DecimalSI),
			},
		},
		{
			info: provider.ExternalMetricInfo{
				Metric: queue_metric_name,
			},
			labels: map[string]string{},
			value: external_metrics.ExternalMetricValue{
				MetricName:   queue_metric_name,
				MetricLabels: map[string]string{},
				Value:        *resource.NewQuantity(getQueueMessagesValue(), resource.DecimalSI),
			},
		},
	}
)

func getQueueMessagesValue() int64 {
	var messagesValue int64
	request, err := http.NewRequest("GET", "http://172.16.106.194:15672/api/queues/%2f/spring-boot", nil)
	if err == nil {
		cli := &http.Client{}
		request.SetBasicAuth(userid, password)
		response, err := cli.Do(request)
		if err == nil {
			decoder := json.NewDecoder(response.Body)
			var metrics Queue_Metrics
			err = decoder.Decode(&metrics)
			if err == nil {
				fmt.Printf("Number of messages on the queue: %d\n", metrics.Messages)
				fmt.Printf("Queue name: %s\n", metrics.Name)
				messagesValue = metrics.Messages
			} else {
				fmt.Printf("There was an error parsing json content of RabbitMQ message: %s\n", err)
			}
		} else {
			fmt.Printf("The HTTP request to RabbitMq endpoint failed with error: %s\n", err)
		}
	} else {
		fmt.Printf("There was an error instantiating http request to RabbitMQ endpoint: %s\n", err)
	}
	return messagesValue
}

func NewFakeProvider(client dynamic.Interface, mapper apimeta.RESTMapper) provider.MetricsProvider {
	return &testingProvider{
		client:          client,
		mapper:          mapper,
		values:          make(map[provider.CustomMetricInfo]int64),
		externalMetrics: testingMetrics,
	}
}

func (p *testingProvider) valueFor(info provider.CustomMetricInfo) (int64, error) {
	info, _, err := info.Normalized(p.mapper)
	if err != nil {
		return 0, err
	}
	var value int64 = 42
	response, err := http.Get("http://httpbin.org/ip")
	if err == nil {
		var jsonIpOrigin IP_Origin
		decoder := json.NewDecoder(response.Body)
		err = decoder.Decode(&jsonIpOrigin)
		if err == nil {
			ip_numbers := strings.Split(jsonIpOrigin.Origin, ".")
			max_number, _ := strconv.Atoi(ip_numbers[0])
			for i := 1; i <= 3; i++ {
				temp_max_number, _ := strconv.Atoi(ip_numbers[i])
				if temp_max_number > max_number {
					max_number = temp_max_number
				}
			}
			value = int64(max_number)
		} else {
			fmt.Printf("There was an error while decoding response from httpbin.org/ip: %s\n", err)
		}
	} else {
		fmt.Printf("There was an error while getting response from httpbin.org/ip: %s\n", err)
	}
	p.values[info] = value

	return value, nil
}

func (p *testingProvider) metricFor(value int64, name types.NamespacedName, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	objRef, err := helpers.ReferenceFor(p.mapper, name, info)
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: objRef,
		MetricName:      info.Metric,
		Timestamp:       metav1.Time{time.Now()},
		Value:           *resource.NewQuantity(value, resource.DecimalSI),
	}, nil
}

func (p *testingProvider) metricsFor(totalValue int64, namespace string, selector labels.Selector, info provider.CustomMetricInfo) (*custom_metrics.MetricValueList, error) {
	names, err := helpers.ListObjectNames(p.mapper, p.client, namespace, selector, info)
	if err != nil {
		return nil, err
	}

	res := make([]custom_metrics.MetricValue, len(names))
	for i, name := range names {
		value, err := p.metricFor(totalValue/int64(len(res)), types.NamespacedName{Namespace: namespace, Name: name}, info)
		if err != nil {
			return nil, err
		}
		res[i] = *value
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *testingProvider) GetMetricByName(name types.NamespacedName, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	value, err := p.valueFor(info)
	if err != nil {
		return nil, err
	}
	return p.metricFor(value, name, info)
}

func (p *testingProvider) GetMetricBySelector(namespace string, selector labels.Selector, info provider.CustomMetricInfo) (*custom_metrics.MetricValueList, error) {
	totalValue, err := p.valueFor(info)
	if err != nil {
		return nil, err
	}

	return p.metricsFor(totalValue, namespace, selector, info)
}

func (p *testingProvider) ListAllMetrics() []provider.CustomMetricInfo {
	// TODO: maybe dynamically generate this?
	return []provider.CustomMetricInfo{
		// these are mostly arbitrary examples
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Metric:        "packets-per-second",
			Namespaced:    true,
		},
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "services"},
			Metric:        "connections-per-second",
			Namespaced:    true,
		},
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "namespaces"},
			Metric:        "work-queue-length",
			Namespaced:    false,
		},
	}
}
func (p *testingProvider) GetExternalMetric(namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	matchingMetrics := []external_metrics.ExternalMetricValue{}
	for _, metric := range p.externalMetrics {
		if metric.info.Metric == info.Metric && metricSelector.Matches(labels.Set(metric.labels)) {
			if info.Metric == queue_metric_name {
				metric.value.Value = *resource.NewQuantity(getQueueMessagesValue(), resource.DecimalSI)
			}
			metricValue := metric.value
			metricValue.Timestamp = metav1.Now()
			matchingMetrics = append(matchingMetrics, metricValue)
		}
	}
	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

func (p *testingProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	externalMetricsInfo := []provider.ExternalMetricInfo{}
	for _, metric := range p.externalMetrics {
		externalMetricsInfo = append(externalMetricsInfo, metric.info)
	}
	return externalMetricsInfo
}
