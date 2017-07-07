// Copyright Piero de Salvia.
// All Rights Reserved

package pluginator

import (
	"log"
	"strconv"
	"time"

	"github.com/hashicorp/consul/api"
)

type consulWatcher struct {
	prefix    string
	Events    chan consulEvent
	KVClient  *api.KV
	kvS       map[string]*valueAndModified
	terminate bool
}

type consulEvent struct {
	Action consulAction
	Key    string
	Value  string
}

type consulAction string

const (
	consulAddAction    consulAction = "Add"
	consulRemoveAction consulAction = "Remove"
	consulUpdateAction consulAction = "Update"
)

type valueAndModified struct {
	Value    string
	Modified uint64
}

func newConsulWatcher(host string, port int, keyPrefix string) (*consulWatcher, error) {

	cw := consulWatcher{}

	config := api.DefaultConfig()
	(*config).Address = host + ":" + strconv.Itoa(port)

	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}
	kv := client.KV()

	cw.prefix = keyPrefix
	cw.Events = make(chan consulEvent)
	cw.KVClient = kv
	cw.kvS = make(map[string]*valueAndModified)

	go func() {
		for !cw.terminate {
			time.Sleep(3 * time.Second)
			cw.scan()
		}
	}()

	return &cw, nil
}

func (cw *consulWatcher) Terminate() {
	cw.terminate = true
	log.Println("Terminating consul watcher...")
}

func (cw *consulWatcher) scan() {

	kvList, _, err := cw.KVClient.List(cw.prefix, nil)
	if err != nil {
		log.Println(err)
		return
	}
	for _, kvPair := range kvList {
		if vM, exists := cw.kvS[kvPair.Key]; !exists {
			vM := valueAndModified{
				Value:    string(kvPair.Value),
				Modified: kvPair.ModifyIndex,
			}
			cw.kvS[kvPair.Key] = &vM
			event := consulEvent{
				Action: consulAddAction,
				Key:    kvPair.Key,
				Value:  string(kvPair.Value),
			}
			cw.Events <- event
		} else {
			if kvPair.ModifyIndex > vM.Modified {
				vM := valueAndModified{
					Value:    string(kvPair.Value),
					Modified: kvPair.ModifyIndex,
				}
				cw.kvS[kvPair.Key] = &vM
				event := consulEvent{
					Action: consulUpdateAction,
					Key:    kvPair.Key,
					Value:  string(kvPair.Value),
				}
				cw.Events <- event
			}
		}
	}
	for k, vm := range cw.kvS {
		if !contains(kvList, k) {
			event := consulEvent{
				Action: consulRemoveAction,
				Key:    k,
				Value:  vm.Value,
			}
			cw.Events <- event
			delete(cw.kvS, k)
		}
	}
}

func contains(slice []*api.KVPair, key string) bool {
	found := false
	for _, kvPair := range slice {
		found = (*kvPair).Key == key
		if found {
			break
		}
	}
	return found
}
