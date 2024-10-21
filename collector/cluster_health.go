// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	colors                     = []string{"green", "yellow", "red"}
	defaultClusterHealthLabels = []string{"cluster"}
)

type clusterHealthMetric struct {
	Type  prometheus.ValueType
	Desc  *prometheus.Desc
	Value func(clusterHealth clusterHealthResponse) float64
}

type clusterHealthStatusMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(clusterHealth clusterHealthResponse, color string) float64
	Labels func(clusterName, color string) []string
}

// ClusterHealth type defines the collector struct
type ClusterHealth struct {
	logger log.Logger
	client *http.Client
	url    *url.URL

	// Prometheus descriptors for Kibana rule execution statuses
	kibanaSucceededMetric      *prometheus.Desc
	kibanaPartialFailureMetric *prometheus.Desc
	kibanaFailureMetric        *prometheus.Desc

	metrics      []*clusterHealthMetric
	statusMetric *clusterHealthStatusMetric
}

// NewClusterHealth returns a new Collector exposing ClusterHealth stats.
func NewClusterHealth(logger log.Logger, client *http.Client, url *url.URL) *ClusterHealth {
	subsystem := "cluster_health"

	return &ClusterHealth{
		logger: logger,
		client: client,
		url:    url,

		kibanaSucceededMetric: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "kibana_rule_succeeded_total"),
			"Total number of Kibana rules that succeeded.",
			[]string{"cluster"}, nil,
		),
		kibanaPartialFailureMetric: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "kibana_rule_partial_failure_total"),
			"Total number of Kibana rules with partial failures.",
			[]string{"cluster"}, nil,
		),
		kibanaFailureMetric: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "kibana_rule_failed_total"),
			"Total number of Kibana rules that failed.",
			[]string{"cluster"}, nil,
		),

		metrics: []*clusterHealthMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "active_primary_shards"),
					"The number of primary shards in your cluster. This is an aggregate total across all indices.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.ActivePrimaryShards)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "active_shards"),
					"Aggregate total of all shards across all indices, which includes replica shards.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.ActiveShards)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "delayed_unassigned_shards"),
					"Shards delayed to reduce reallocation overhead",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.DelayedUnassignedShards)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "initializing_shards"),
					"Count of shards that are being freshly created.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.InitializingShards)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "number_of_data_nodes"),
					"Number of data nodes in the cluster.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.NumberOfDataNodes)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "number_of_in_flight_fetch"),
					"The number of ongoing shard info requests.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.NumberOfInFlightFetch)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "task_max_waiting_in_queue_millis"),
					"Tasks max time waiting in queue.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.TaskMaxWaitingInQueueMillis)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "number_of_nodes"),
					"Number of nodes in the cluster.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.NumberOfNodes)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "number_of_pending_tasks"),
					"Cluster level changes which have not yet been executed",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.NumberOfPendingTasks)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "relocating_shards"),
					"The number of shards that are currently moving from one node to another node.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.RelocatingShards)
				},
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "unassigned_shards"),
					"The number of shards that exist in the cluster state, but cannot be found in the cluster itself.",
					defaultClusterHealthLabels, nil,
				),
				Value: func(clusterHealth clusterHealthResponse) float64 {
					return float64(clusterHealth.UnassignedShards)
				},
			},
		},
		statusMetric: &clusterHealthStatusMetric{
			Type: prometheus.GaugeValue,
			Desc: prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "status"),
				"Whether all primary and replica shards are allocated.",
				[]string{"cluster", "color"}, nil,
			),
			Value: func(clusterHealth clusterHealthResponse, color string) float64 {
				if clusterHealth.Status == color {
					return 1
				}
				return 0
			},
		},
	}
}

// Describe set Prometheus metrics descriptions.
func (c *ClusterHealth) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.Desc
	}
	ch <- c.statusMetric.Desc
}

func (c *ClusterHealth) fetchAndDecodeClusterHealth() (clusterHealthResponse, error) {
	var chr clusterHealthResponse

	u := *c.url
	u.Path = path.Join(u.Path, "/_cluster/health")
	res, err := c.client.Get(u.String())
	if err != nil {
		return chr, fmt.Errorf("failed to get cluster health from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			level.Warn(c.logger).Log(
				"msg", "failed to close http.Client",
				"err", err,
			)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return chr, fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return chr, err
	}

	if err := json.Unmarshal(bts, &chr); err != nil {
		return chr, err
	}

	return chr, nil
}

// Collect collects ClusterHealth metrics.
func (c *ClusterHealth) Collect(ch chan<- prometheus.Metric) {
	clusterHealthResp, err := c.fetchAndDecodeClusterHealth()
	if err != nil {
		level.Warn(c.logger).Log(
			"msg", "failed to fetch and decode cluster health",
			"err", err,
		)
		return
	}

	for _, metric := range c.metrics {
		ch <- prometheus.MustNewConstMetric(
			metric.Desc,
			metric.Type,
			metric.Value(clusterHealthResp),
			clusterHealthResp.ClusterName,
		)
	}

	for _, color := range colors {
		ch <- prometheus.MustNewConstMetric(
			c.statusMetric.Desc,
			c.statusMetric.Type,
			c.statusMetric.Value(clusterHealthResp, color),
			clusterHealthResp.ClusterName, color,
		)
	}

	// Fetch Kibana rule execution statuses
	succeeded, partialFailures, failures, err := c.fetchKibanaRuleExecutionStatus()
	if err != nil {
		level.Warn(c.logger).Log(
			"msg", "failed to fetch Kibana rule execution statuses",
			"err", err,
		)
		return
	}

	// Expose Kibana rule metrics as Prometheus metrics
	ch <- prometheus.MustNewConstMetric(
		c.kibanaSucceededMetric,
		prometheus.GaugeValue,
		float64(succeeded),
		clusterHealthResp.ClusterName,
	)

	ch <- prometheus.MustNewConstMetric(
		c.kibanaPartialFailureMetric,
		prometheus.GaugeValue,
		float64(partialFailures),
		clusterHealthResp.ClusterName,
	)

	ch <- prometheus.MustNewConstMetric(
		c.kibanaFailureMetric,
		prometheus.GaugeValue,
		float64(failures),
		clusterHealthResp.ClusterName,
	)

}

// Query Kibana Logs for Rule Execution Metrics - this must be done as a POST query
func (c *ClusterHealth) fetchKibanaRuleExecutionStatus() (succeeded, partialFailures, failures int, err error) {
	var result struct {
		Aggregations struct {
			Statuses struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int    `json:"doc_count"`
				} `json:"buckets"`
			} `json:"statuses"`
		} `json:"aggregations"`
	}

	// Define the Elasticsearch query
	query := `{
		"query": {
			"bool": {
				"should": [
					{ "match": { "kibana.alert.rule.execution.status": "succeeded" } },
					{ "match": { "kibana.alert.rule.execution.status": "partial failure" } },
					{ "match": { "kibana.alert.rule.execution.status": "failed" } }
				]
			}
		},
		"aggs": {
			"statuses": {
				"terms": {
					"field": "kibana.alert.rule.execution.status"
				}
			}
		},
		"size": 0
	}`

	// Perform the request to Elasticsearch
	u := *c.url
	u.Path = path.Join(u.Path, "/.kibana-event-log-*/_search")
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(query))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to create request to Elasticsearch: %s", err)
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to fetch Kibana rule execution status: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("Elasticsearch API request failed with code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, 0, 0, err
	}

	// Parse the JSON response
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, 0, 0, err
	}

	// Loop through buckets to count the different statuses
	for _, bucket := range result.Aggregations.Statuses.Buckets {
		switch bucket.Key {
		case "succeeded":
			succeeded = bucket.DocCount
		case "partial failure":
			partialFailures = bucket.DocCount
		case "failed":
			failures = bucket.DocCount
		}
	}

	return succeeded, partialFailures, failures, nil
}
