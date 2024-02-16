/*
Copyright 2016 The Kubernetes Authors.

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

// This file provides abstractions for setting the provider (e.g., prometheus)
// of metrics.

package cache

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	testingclock "k8s.io/utils/clock/testing"
	"sync"
	"testing"
	"time"
)

type fakeMetric struct {
	inc int64
	set float64

	observedValue float64
	observedCount int

	lock sync.Mutex
}

func (m *fakeMetric) Inc() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.inc++
}
func (m *fakeMetric) Observe(v float64) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.observedValue = v
	m.observedCount++
}
func (m *fakeMetric) Set(v float64) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.set = v
}

func (m *fakeMetric) countValue() int64 {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.inc
}

func (m *fakeMetric) gaugeValue() float64 {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.set
}

func (m *fakeMetric) observationValue() float64 {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.observedValue
}

func (m *fakeMetric) observationCount() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.observedCount
}

func (m *fakeMetric) cleanUp() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.inc = 0
	m.set = 0
	m.observedValue = 0
	m.observedCount = 0
}

func newFakeMetric() *fakeMetric {
	return &fakeMetric{}
}

type fakeMetricsProvider struct {
	numberOfLists        *fakeMetric
	listDuration         *fakeMetric
	numberOfItemsInList  *fakeMetric
	numberOfWatchLists   *fakeMetric
	numberOfWatches      *fakeMetric
	numberOfShortWatches *fakeMetric
	watchDuration        *fakeMetric
	numberOfItemsInWatch *fakeMetric
	lastResourceVersion  *fakeMetric
}

func (m *fakeMetricsProvider) NewListsMetric(name string) CounterMetric {
	return m.numberOfLists
}
func (m *fakeMetricsProvider) DeleteListsMetric(name string) {
	m.numberOfLists.cleanUp()
}
func (m *fakeMetricsProvider) NewListDurationMetric(name string) HistogramMetric {
	return m.listDuration
}
func (m *fakeMetricsProvider) DeleteListDurationMetric(name string) {
	m.listDuration.cleanUp()
}
func (m *fakeMetricsProvider) NewItemsInListMetric(name string) HistogramMetric {
	return m.numberOfItemsInList
}
func (m *fakeMetricsProvider) DeleteItemsInListMetric(name string) {
	m.numberOfItemsInList.cleanUp()
}
func (m *fakeMetricsProvider) NewWatchListsMetrics(name string) CounterMetric {
	return m.numberOfWatchLists
}
func (m *fakeMetricsProvider) DeleteWatchListsMetrics(name string) {
	m.numberOfWatchLists.cleanUp()
}
func (m *fakeMetricsProvider) NewWatchesMetric(name string) CounterMetric {
	return m.numberOfWatches
}
func (m *fakeMetricsProvider) DeleteWatchesMetric(name string) {
	m.numberOfWatches.cleanUp()
}
func (m *fakeMetricsProvider) NewShortWatchesMetric(name string) CounterMetric {
	return m.numberOfShortWatches
}
func (m *fakeMetricsProvider) DeleteShortWatchesMetric(name string) {
	m.numberOfShortWatches.cleanUp()
}
func (m *fakeMetricsProvider) NewWatchDurationMetric(name string) HistogramMetric {
	return m.watchDuration
}
func (m *fakeMetricsProvider) DeleteWatchDurationMetric(name string) {
	m.watchDuration.cleanUp()
}
func (m *fakeMetricsProvider) NewItemsInWatchMetric(name string) HistogramMetric {
	return m.numberOfItemsInWatch
}
func (m *fakeMetricsProvider) DeleteItemsInWatchMetric(name string) {
	m.numberOfItemsInWatch.cleanUp()
}
func (m *fakeMetricsProvider) NewLastResourceVersionMetric(name string) GaugeMetric {
	return m.lastResourceVersion
}
func (m *fakeMetricsProvider) DeleteLastResourceVersionMetric(name string) {
	m.lastResourceVersion.cleanUp()
}

func TestReflectorRunMetrics(t *testing.T) {
	fakeClock := testingclock.NewFakeClock(time.Unix(0, 0))

	metricsProvider := &fakeMetricsProvider{
		numberOfLists:        newFakeMetric(),
		listDuration:         newFakeMetric(),
		numberOfItemsInList:  newFakeMetric(),
		numberOfWatchLists:   newFakeMetric(),
		numberOfWatches:      newFakeMetric(),
		numberOfShortWatches: newFakeMetric(),
		watchDuration:        newFakeMetric(),
		numberOfItemsInWatch: newFakeMetric(),
		lastResourceVersion:  newFakeMetric(),
	}
	SetReflectorMetricsProvider(metricsProvider)

	stopCh := make(chan struct{})
	store := NewStore(MetaNamespaceKeyFunc)
	r := NewReflectorWithOptions(&testLW{}, &v1.Pod{}, store, ReflectorOptions{
		ResyncPeriod: 0,
		Clock:        fakeClock,
	})
	var fw *watch.FakeWatcher
	numberOfWatches := 0
	r.listerWatcher = &testLW{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			numberOfWatches++
			fw = watch.NewFake()
			return fw, nil
		},
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			fakeClock.Step(time.Millisecond)
			return &v1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}}, nil
		},
	}
	go r.Run(stopCh)
	err := wait.PollUntilContextTimeout(context.TODO(), time.Microsecond, time.Second, true, func(ctx context.Context) (bool, error) {
		return fw != nil, nil
	})
	if err != nil {
		t.Fatalf("timeout to wait reflector watch")
	}
	fw.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bar", ResourceVersion: "3"}})
	// ensure the add event has been handled
	err = wait.PollUntilContextTimeout(context.TODO(), time.Microsecond, time.Second, true, func(ctx context.Context) (bool, error) {
		return r.lastSyncResourceVersion == "3", nil
	})
	if err != nil {
		t.Fatalf("timeout to wait event handled")
	}

	verifyMetricCountValue(t, metricsProvider.numberOfLists, 1, "numberOfLists")
	verifyMetricHistogramValue(t, metricsProvider.listDuration, 0.001, "listDuration")
	verifyMetricHistogramValue(t, metricsProvider.numberOfItemsInList, 0, "numberOfItemsInList")
	verifyMetricCountValue(t, metricsProvider.numberOfWatches, 1, "numberOfWatches")
	verifyMetricGaugeValue(t, metricsProvider.lastResourceVersion, 3, "lastResourceVersion")

	// stop watch to verify metrics numberOfItemsInWatch
	fw.Stop()
	err = wait.PollUntilContextTimeout(context.TODO(), time.Microsecond, time.Second, true, func(ctx context.Context) (bool, error) {
		return numberOfWatches > 1, nil
	})
	if err != nil {
		t.Fatalf("timeout to wait reflector watch loop")
	}
	verifyMetricHistogramValue(t, metricsProvider.numberOfItemsInWatch, 1, "numberOfItemsInWatch")

	close(stopCh)

	//numberOfLists        CounterMetric
	//listDuration         HistogramMetric
	//numberOfItemsInList  HistogramMetric
	//numberOfWatchLists   CounterMetric
	//numberOfWatches      CounterMetric
	//numberOfShortWatches CounterMetric
	//watchDuration        HistogramMetric
	//numberOfItemsInWatch HistogramMetric
	//lastResourceVersion  GaugeMetric

	//ensure reflector metrics are deleted
	err = wait.PollUntilContextTimeout(context.TODO(), time.Microsecond, time.Second, true, func(ctx context.Context) (bool, error) {
		return metricsProvider.numberOfLists.countValue() == 0 &&
			metricsProvider.listDuration.observedCount == 0 &&
			metricsProvider.numberOfItemsInList.observedCount == 0 &&
			metricsProvider.numberOfWatchLists.countValue() == 0 &&
			metricsProvider.numberOfWatches.countValue() == 0 &&
			metricsProvider.numberOfShortWatches.countValue() == 0 &&
			metricsProvider.watchDuration.observedCount == 0 &&
			metricsProvider.numberOfItemsInWatch.observedCount == 0 &&
			metricsProvider.lastResourceVersion.gaugeValue() == 0, nil
	})
	if err != nil {
		t.Fatalf("timeout to wait reflector metrics cleanUp")
	}
}

func verifyMetricCountValue(t *testing.T, m *fakeMetric, expect int64, metricName string) {
	if got := m.countValue(); got != expect {
		t.Errorf("reflector metric %s metric expected %v, got %v", metricName, expect, got)
	}
}

func verifyMetricHistogramValue(t *testing.T, m *fakeMetric, expect float64, metricName string) {
	if got := m.observationValue(); got != expect {
		t.Errorf("reflector metric %s metric expected %v, got %v", metricName, expect, got)
	}
}

func verifyMetricGaugeValue(t *testing.T, m *fakeMetric, expect float64, metricName string) {
	if got := m.gaugeValue(); got != expect {
		t.Errorf("reflector metric %s metric expected %v, got %v", metricName, expect, got)
	}
}
