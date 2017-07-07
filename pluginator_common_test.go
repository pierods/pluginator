// Copyright Piero de Salvia.
// All Rights Reserved
package pluginator

import (
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

var (
	testDataDir    string
	runConsulTests = flag.Bool("consul", false, "whether to run tests requiring a consul instance running at localhost:8500")
)

func init() {

	goPath := os.Getenv("GOPATH")
	testDataDir = goPath + "/src/github.com/pierods/pluginator/testdata"
}

func TestMain(m *testing.M) {

	flag.Parse()
	retCode := m.Run()
	os.Exit(retCode)
}

func copyTestFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if stats, _ := os.Stat(dst); stats != nil {
		return errors.New(dst + " already exists")
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

func deleteTestFile(fileName string) error {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return errors.New(fileName + " does not exist")
	}
	return os.Remove(fileName)
}

func updateTestFile(fileName, content string) error {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return errors.New(fileName + " does not exist")
	}
	return ioutil.WriteFile(fileName, []byte(content), 700)
}

func readTestFile(fileName string) (string, error) {
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		return "", errors.New(fileName + " does not exist")
	}

	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

type EventSubscriber struct {
	ScanDone       chan bool
	RemoveDone     chan bool
	UpdateDone     chan bool
	AddDone        chan bool
	AddedName      string
	AddedLib       *PluginContent
	RemovedName    string
	RemovedLib     *PluginContent
	ScannedPlugins map[string]*PluginContent
	UpdatedName    string
	UpdatedLib     *PluginContent
}

func (e *EventSubscriber) ScanSubscriber(pluginNamesAndLibs map[string]*PluginContent) {
	e.ScannedPlugins = pluginNamesAndLibs
	go func() {
		e.ScanDone <- true
	}()

}

func (e *EventSubscriber) RemoveSubscriber(pluginName string, pluginLib *PluginContent) {
	e.RemovedName = pluginName
	e.RemovedLib = pluginLib
	go func() {
		e.RemoveDone <- true
	}()
}

func (e *EventSubscriber) UpdateSubscriber(pluginName string, pluginLib *PluginContent) {
	e.UpdatedName = pluginName
	e.UpdatedLib = pluginLib
	go func() {
		e.UpdateDone <- true
	}()
}

func (e *EventSubscriber) AddSubscriber(pluginName string, pluginLib *PluginContent) {
	e.AddedName = pluginName
	e.AddedLib = pluginLib
	go func() {
		e.AddDone <- true
	}()
}
