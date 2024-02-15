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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sync"
	"testing"
	"time"
)

type fakeMetric struct {
	inc int64
	set float64

	observedValue float64
	observedCount int

	notifyCh chan struct{}

	lock sync.Mutex
}

func (m *fakeMetric) Inc() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.inc++
	m.notify()
}
func (m *fakeMetric) Observe(v float64) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.observedValue = v
	m.observedCount++
	m.notify()
}
func (m *fakeMetric) Set(v float64) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.set = v
	m.notify()
}

func (m *fakeMetric) notify() {
	if m.notifyCh != nil {
		m.notifyCh <- struct{}{}
	}
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
	return &fakeMetric{
		notifyCh: make(chan struct{}, 9),
	}
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
	r := NewReflector(&testLW{}, &v1.Pod{}, store, 0)
	fw := watch.NewFake()
	r.listerWatcher = &testLW{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return fw, nil
		},
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return &v1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}}, nil
		},
	}
	go r.Run(stopCh)
	fw.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bar"}})

	select {
	// use a channel to ensure we don't look at the metric before it's
	// been set.
	case _, ok := <-metricsProvider.numberOfLists.notifyCh:
		if ok {
			if metricsProvider.numberOfLists.countValue() != 1 {
				t.Errorf("reflector numberOfLists metric is not 1 ")
			} else {

			}

		} else {

		}

	case <-time.After(time.Second):
		t.Errorf("wait metric timeout")
		break
	}
	close(stopCh)

}
