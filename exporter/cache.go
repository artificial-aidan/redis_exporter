package exporter

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type MetricCache struct {
	sync.Mutex

	holdoff *time.Ticker
	cache   []prometheus.Metric
	started bool
}

func (e *Exporter) cacheMetric(m prometheus.Metric) {
	e.cache.cache = append(e.cache.cache, m)
}

func (e *Exporter) intercept(in <-chan prometheus.Metric, out chan<- prometheus.Metric, wg *sync.WaitGroup) {
	for m := range in {
		e.cacheMetric(m)
		out <- m
	}
	log.Info("Done intercepting")
	wg.Done()
}

func (e *Exporter) doCache(ch chan<- prometheus.Metric) {
	e.cache.Lock()
	defer e.cache.Unlock()
	// Bust cache by setting to 0. Don't GC slice
	e.cache.cache = e.cache.cache[:0]
	// Man in the middle channel
	mim := make(chan prometheus.Metric)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	// Start interceptor
	go e.intercept(mim, ch, wg)
	// Call collect with mim channel
	e.collect(mim)

	fmt.Println(len(mim))
	// Wait for mim channel to drain
	close(mim)
	wg.Wait()
}

func (e *Exporter) cachedCollect(ch chan<- prometheus.Metric) {
	if !e.cache.started {
		e.cache.started = true
		e.doCache(ch)
		return
	}

	select {
	// Only collect metrics if we have hit the ticker
	case <-e.cache.holdoff.C:
		log.Debug("Holdoff timer ticked, collecting metrics")
		log.Info("Holdoff timer ticked, collecting metrics")
		e.doCache(ch)
		break
	default:
		log.Info("Skipping metric collection because of holdoff timer")
		log.Debug("Skipping metric collection because of holdoff timer")
		e.totalCacheHits.Inc()
		for _, m := range e.cache.cache {
			ch <- m
		}
		ch <- e.totalCacheHits
		return
	}
}
