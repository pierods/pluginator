// Copyright Piero de Salvia.
// All Rights Reserved
package pluginator

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/consul/api"
)

func TestConsulWatcher(t *testing.T) {

	if !*runConsulTests {
		t.SkipNow()
	}
	config := api.DefaultConfig()
	(*config).Address = "localhost:8500"

	client, err := api.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	kv := client.KV()

	uuid := uuid.New().String()

	cw, err := NewConsulWatcher("localhost", 8500, uuid)
	if err != nil {
		t.Fatal(err)
	}

	p := &api.KVPair{
		Key:   uuid + ".key1",
		Value: []byte("key 1 bytes"),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-cw.Events:
		if event.Action != consulAddAction {
			t.Fatal("Should be able to detect an added key/value pair")
		}
		if event.Key != uuid+".key1" {
			t.Fatal("Should be able to read an added key")
		}
		if event.Value != "key 1 bytes" {
			t.Fatal("Should be able to read an added value")
		}
	}
	p = &api.KVPair{
		Key:   uuid + ".key2",
		Value: []byte("key 2 bytes"),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-cw.Events:
		if event.Action != consulAddAction {
			t.Fatal("Should be able to detect an added key/value pair")
		}
		if event.Key != uuid+".key2" {
			t.Fatal("Should be able to read an added key")
		}
		if event.Value != "key 2 bytes" {
			t.Fatal("Should be able to read an added value")
		}
	}

	p = &api.KVPair{
		Key:   uuid + ".key1",
		Value: []byte("key 1 bytes-updated"),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-cw.Events:
		if event.Action != consulUpdateAction {
			t.Fatal("Should be able to detect an updated key/value pair")
		}
		if event.Key != uuid+".key1" {
			t.Fatal("Should be able to read an updated key")
		}
		if event.Value != "key 1 bytes-updated" {
			t.Fatal("Should be able to read an updated value")
		}
	}

	_, err = kv.Delete(uuid+".key2", nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-cw.Events:
		if event.Action != consulRemoveAction {
			t.Fatal("Should be able to detect a removed key/value pair")
		}
		if event.Key != uuid+".key2" {
			t.Fatal("Should be able to read a removed key")
		}
		if event.Value != "key 2 bytes" {
			t.Fatal("Should be able to read a deleted value")
		}
	}

}

func TestConsulMode(t *testing.T) {

	if !*runConsulTests {
		t.SkipNow()
	}

	var err error
	tempPluginDir, err := ioutil.TempDir("", "testconsulplugindir")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	t.Log("tmpPluginDir=", tempPluginDir)
	config := api.DefaultConfig()
	(*config).Address = "localhost:8500"

	client, err := api.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	uuid := uuid.New().String()
	pluginator, err := NewPluginatorC("localhost", 8500, uuid)
	if err != nil {
		t.Fatal(err)
	}

	plugin1, err := ioutil.ReadFile(testDataDir + "/plugin1.go")
	if err != nil {
		t.Fatal(err)
	}
	plugin2, err := ioutil.ReadFile(testDataDir + "/plugin2.go")
	if err != nil {
		t.Fatal(err)
	}

	plugin3, err := ioutil.ReadFile(testDataDir + "/plugin3.go")
	if err != nil {
		t.Fatal(err)
	}

	kv := client.KV()
	p := &api.KVPair{
		Key:   uuid + ".plugin1.go",
		Value: []byte(plugin1),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}

	p = &api.KVPair{
		Key:   uuid + ".plugin2.go",
		Value: []byte(plugin2),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	es := EventSubscriber{
		ScanDone:   make(chan bool),
		RemoveDone: make(chan bool),
		UpdateDone: make(chan bool),
		AddDone:    make(chan bool),
	}

	pluginator.SubscribeScan(es.ScanSubscriber)

	err = pluginator.Start()
	if err != nil {
		t.Fatal(err)
	}
	// give consul watcher time to poll consull
	time.Sleep(10 * time.Second)
	select {
	case _ = <-es.ScanDone:
		if len(es.ScannedPlugins) != 2 {
			t.Fatal("Should be able to scan plugins")
		}
		p1, exists := es.ScannedPlugins["plugin1"]
		if !exists {
			t.Fatal("Should be able to load a plugin")
		}
		p2, exists := es.ScannedPlugins["plugin2"]
		if !exists {
			t.Fatal("Should be able to load a plugin")
		}
		addPtr, err := p1.Lib.Lookup("Add")
		if err != nil {
			t.Fatal("Should be able to lookup a symbol")
		}
		subPtr, err := p2.Lib.Lookup("Sub")
		if err != nil {
			t.Fatal("Should be able to lookup a symbol")
		}
		add, ok := addPtr.(func(int, int) int)
		if !ok {
			t.Fatal("Should be able to convert to function type")
		}
		sub, ok := subPtr.(func(int, int) int)
		if !ok {
			t.Fatal("Should be able to convert to function type")
		}
		sum := add(1, 2)
		if sum != 3 {
			t.Fatal("Should be able to invoke loaded function")
		}
		diff := sub(1, 2)
		if diff != -1 {
			t.Fatal("Should be able to invoke loaded function")
		}
	}

	pluginator.SubscribeRemove(es.RemoveSubscriber)
	_, err = kv.Delete(uuid+".plugin1.go", nil)
	if err != nil {
		t.Fatal("Should be able to remove a test key")
	}

	select {
	case _ = <-es.RemoveDone:
		if es.RemovedName != "plugin1" {
			t.Fatal("Should be able to remove a plugin")
		}
		addPtr, err := es.RemovedLib.Lib.Lookup("Add")
		if err != nil {
			t.Fatal("Should be able to lookup a symbol")
		}
		add, ok := addPtr.(func(int, int) int)
		if !ok {
			t.Fatal("Should be able to convert to function type")
		}
		sum := add(1, 2)
		if sum != 3 {
			t.Fatal("Should be able to invoke loaded function")
		}
	}
	pluginator.SubscribeUpdate(es.UpdateSubscriber)

	p = &api.KVPair{
		Key:   uuid + ".plugin2.go",
		Value: []byte(plugin1),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	// plugin2 now contains Add
	select {
	case _ = <-es.UpdateDone:
		if es.UpdatedName != "plugin2" {
			t.Fatal("Should be able to update a plugin")
		}
		addPtr, err := es.UpdatedLib.Lib.Lookup("Add")
		if err != nil {
			t.Log(err)
			t.Fatal("Should be able to lookup a symbol")
		}
		add, ok := addPtr.(func(int, int) int)
		if !ok {
			t.Fatal("Should be able to convert to function type")
		}
		sum := add(1, 2)
		if sum != 3 {
			t.Fatal("Should be able to invoke loaded function")
		}
	}
	pluginator.SubscribeAdd(es.AddSubscriber)

	p = &api.KVPair{
		Key:   uuid + ".plugin3.go",
		Value: []byte(plugin3),
	}

	_, err = kv.Put(p, nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case _ = <-es.AddDone:
		if es.AddedName != "plugin3" {
			t.Fatal("Should be able to load an added file")
		}
		mulPtr, err := es.AddedLib.Lib.Lookup("Mul")
		if err != nil {
			t.Log(err)
			t.Fatal("Should be able to lookup a symbol")
		}
		mul, ok := mulPtr.(func(int, int) int)
		if !ok {
			t.Fatal("Should be able to convert to function type")
		}
		prod := mul(3, 2)
		if prod != 6 {
			t.Fatal("Should be able to invoke loaded function")
		}
	}
	pluginator.Terminate()

}
