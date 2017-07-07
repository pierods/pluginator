package pluginator

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestPluginator(t *testing.T) {

	var err error
	tempPluginDir, err := ioutil.TempDir("", "testplugindir")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	pluginator, err := NewPluginatorF(tempPluginDir)
	if err != nil {
		t.Fatal(err)
	}
	err = copyTestFile(tempPluginDir+"/plugin1.go", testDataDir+"/plugin1.go")
	if err != nil {
		t.Fatal(err)
	}
	err = copyTestFile(tempPluginDir+"/plugin2.go", testDataDir+"/plugin2.go")
	if err != nil {
		t.Fatal(err)
	}

	p1Code, err := readTestFile(testDataDir + "/plugin1.go")
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
		if p1.Code != p1Code {
			t.Fatal("Should be able to read a plugins code")
		}
	}

	pluginator.SubscribeRemove(es.RemoveSubscriber)
	err = deleteTestFile(tempPluginDir + "/plugin1.go")
	if err != nil {
		t.Fatal("Should be able to remove a test file")
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

	content, err := readTestFile(testDataDir + "/plugin1.go")
	if err != nil {
		t.Fatal(err)
	}
	err = updateTestFile(tempPluginDir+"/plugin2.go", content)
	// plugin2 now contains Add
	if err != nil {
		t.Fatal(err)
	}

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
	err = copyTestFile(tempPluginDir+"/plugin3.go", testDataDir+"/plugin3.go")
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
