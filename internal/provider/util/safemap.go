package util

import (
	v1 "k8s.io/api/core/v1"
	"sync"
)

type SafeMap struct {
	sync.RWMutex
	Map map[string]*v1.Pod
}

func NewSafeMap() *SafeMap {
	sm := new(SafeMap)
	sm.Map = make(map[string]*v1.Pod)
	return sm
}

func (sm *SafeMap) Read(key string) (*v1.Pod, bool) {
	sm.RLock()
	value, exists := sm.Map[key]
	sm.RUnlock()
	return value, exists
}

func (sm *SafeMap) Write(key string, value *v1.Pod) {
	sm.Lock()
	sm.Map[key] = value
	sm.Unlock()
}

func (sm *SafeMap) Delete(key string) {
	sm.Lock()
	delete(sm.Map, key)
	sm.Unlock()
}
