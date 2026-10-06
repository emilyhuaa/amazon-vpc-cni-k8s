// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package metrics

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-vpc-cni-k8s/pkg/publisher"
	"github.com/aws/amazon-vpc-cni-k8s/pkg/utils/logger"
)

type multiNodeCounterTarget struct {
	order              []string
	addReqCount        map[string]float64
	interestingMetrics map[string]metricsConvert
}

func (t *multiNodeCounterTarget) getLogger() logger.Logger { return logger.DefaultLogger() }

func (t *multiNodeCounterTarget) grabMetricsFromTarget(_ context.Context, target string) ([]byte, error) {
	return []byte(fmt.Sprintf("# TYPE awscni_add_ip_req_count counter\nawscni_add_ip_req_count %g\n", t.addReqCount[target])), nil
}

func (t *multiNodeCounterTarget) getInterestingMetrics() map[string]metricsConvert {
	return t.interestingMetrics
}

func (t *multiNodeCounterTarget) getCWMetricsPublisher() publisher.Publisher { return nil }

func (t *multiNodeCounterTarget) getTargetList(_ context.Context) ([]string, error) {
	return t.order, nil
}

func (t *multiNodeCounterTarget) submitCloudWatch() bool { return false }

func (t *multiNodeCounterTarget) submitPrometheus() bool { return false }

func addReqCountConvert() map[string]metricsConvert {
	return map[string]metricsConvert{
		"awscni_add_ip_req_count": {
			actions: []metricsAction{
				{cwMetricName: "addReqCount", matchFunc: matchAny, actionFunc: metricsAdd, data: &dataPoints{}},
			},
		},
	}
}

func publishedAddReqCount(t *testing.T, target *multiNodeCounterTarget) float64 {
	_, converts, _, err := metricsListGrabAggregateConvert(context.Background(), target)
	assert.NoError(t, err)
	return converts["awscni_add_ip_req_count"].actions[0].data.curSingleDataPoint
}

// The published counter must be the cluster-wide increase since the last poll,
// whichever position the busy node has in the target list.
func TestCounterDeltaIsIndependentOfTargetOrder(t *testing.T) {
	orders := [][]string{
		{"busy", "n2", "n3"},
		{"n2", "busy", "n3"},
		{"n2", "n3", "busy"},
	}
	for _, order := range orders {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			target := &multiNodeCounterTarget{
				order:              order,
				addReqCount:        map[string]float64{"busy": 2, "n2": 5, "n3": 7},
				interestingMetrics: addReqCountConvert(),
			}
			publishedAddReqCount(t, target)
			assert.Equal(t, 0.0, publishedAddReqCount(t, target), "no new ADDs since last poll")

			target.addReqCount["busy"] += 29
			assert.Equal(t, 29.0, publishedAddReqCount(t, target), "29 new ADDs on the busy node")
			assert.Equal(t, 0.0, publishedAddReqCount(t, target), "no new ADDs since last poll")
		})
	}
}

// A restarted aws-node pod reports a lower counter; its new value is the increase.
func TestCounterDeltaAfterTargetRestart(t *testing.T) {
	target := &multiNodeCounterTarget{
		order:              []string{"busy", "n2"},
		addReqCount:        map[string]float64{"busy": 40, "n2": 5},
		interestingMetrics: addReqCountConvert(),
	}
	publishedAddReqCount(t, target)

	target.addReqCount["busy"] = 3
	assert.Equal(t, 3.0, publishedAddReqCount(t, target))
	assert.Equal(t, 0.0, publishedAddReqCount(t, target))
}
