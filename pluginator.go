// Copyright Piero de Salvia.
// All Rights Reserved

// Package pluginator is a low-level plugin manager, working on go source code from the file system or consul
package pluginator

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"plugin"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// PluginContent is sent on pluginator events. It contains the actual library that was loaded and its source code.
type PluginContent struct {
	Lib  *plugin.Plugin
	Code string
}

// Pluginator is lib's entry point
type Pluginator struct {
	pluginDir         string
	watcher           *fsnotify.Watcher
	tempDir           string
	plugins           map[string]*PluginContent
	scanSubscribers   []func(map[string]*PluginContent)
	updateSubscribers []func(string, *PluginContent)
	removeSubscribers []func(string, *PluginContent)
	addSubscribers    []func(string, *PluginContent)
	consulWatcher     *consulWatcher
	consulHost        string
	consulPort        int
	consulKeyPrefix   string
}

// NewPluginatorC instantiates a new Pluginator, watching the subkeys of keyPrefix on the host:port consul instance
func NewPluginatorC(host string, port int, keyPrefix string) (*Pluginator, error) {

	err := checkGoToolchain()
	if err != nil {
		return nil, err
	}

	PluginDir, err := ioutil.TempDir("", "pluginator-consul")
	if err != nil {
		return nil, err
	}
	p := &Pluginator{
		pluginDir:       PluginDir,
		consulHost:      host,
		consulPort:      port,
		consulKeyPrefix: keyPrefix,
		plugins:         make(map[string]*PluginContent),
	}

	p.tempDir, err = ioutil.TempDir("", "pluginator")
	if err != nil {
		return nil, err
	}
	return p, nil

}

// NewPluginatorF instantiates a new Pluginator, watching the PluginDir diretory
func NewPluginatorF(PluginDir string) (*Pluginator, error) {

	err := checkGoToolchain()
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(PluginDir, "/") {
		return nil, errors.New("Plugin dir must not end with /")
	}

	f, err := os.Stat(PluginDir)
	if err != nil {
		return nil, err
	}
	if !f.Mode().IsDir() {
		return nil, errors.New(PluginDir + " is not a directory")
	}

	p := &Pluginator{
		pluginDir: PluginDir,
		plugins:   make(map[string]*PluginContent),
	}

	p.tempDir, err = ioutil.TempDir("", "pluginator")
	if err != nil {
		return nil, err
	}
	return p, nil
}

func checkGoToolchain() error {
	command := exec.Command("go", "version")

	out, err := command.Output()
	if err != nil {
		return err
	}
	outSplit := strings.Split(string(out), " ")
	if len(outSplit) < 4 {
		return errors.New("cannot parse output from go version")
	}

	versionSplit := strings.Split(outSplit[2], ".")
	majV := []rune(versionSplit[0])[2]
	majVI, err := strconv.Atoi(string(majV))
	if err != nil {
		return err
	}

	minV := versionSplit[1]
	minVI, err := strconv.Atoi(minV)
	if err != nil {
		return err
	}

	osVers := outSplit[3]
	if majVI < 1 || minVI < 8 || !strings.HasPrefix(osVers, "linux") {
		return errors.New("Bad go version - need 1.8 or higher on linux")
	}

	return nil
}

// SubscribeScan subscribes its argument to scan events (they happen at start time)
func (p *Pluginator) SubscribeScan(f func(map[string]*PluginContent)) {

	p.scanSubscribers = append(p.scanSubscribers, f)
}

// SubscribeUpdate subscribe its argument to update events (changes in plugin code)
func (p *Pluginator) SubscribeUpdate(f func(string, *PluginContent)) {
	p.updateSubscribers = append(p.updateSubscribers, f)
}

// SubscribeRemove subscribes its argument to remove events (plugin removal)
func (p *Pluginator) SubscribeRemove(f func(string, *PluginContent)) {
	p.removeSubscribers = append(p.removeSubscribers, f)
}

// SubscribeAdd subscribes its argument to add events (plugin adds)
func (p *Pluginator) SubscribeAdd(f func(string, *PluginContent)) {
	p.addSubscribers = append(p.addSubscribers, f)
}

// Start start a Pluginator. It will perform a scan of the watched dir/consul key
func (p *Pluginator) Start() error {
	var msg string
	if p.consulHost != "" {
		msg = p.consulHost + ":" + strconv.Itoa(p.consulPort) + ":" + p.consulKeyPrefix
	} else {
		msg = p.pluginDir
	}
	log.Println("Watching ", msg)
	if p.consulHost != "" {
		var err error
		p.consulWatcher, err = p.watchConsul()
		if err != nil {
			return err
		}
	}

	var err error
	p.watcher, err = p.watch(p.pluginDir)
	if err != nil {
		return err
	}
	p.scan()
	return nil
}

// Terminate makes a Pluginator stop watching a directory/consul key
func (p *Pluginator) Terminate() {
	if p.consulWatcher != nil {
		p.consulWatcher.Terminate()
	}
	err := p.watcher.Close()
	if err != nil {
		log.Println(err)
	}
}

func (p *Pluginator) watch(fileName string) (*fsnotify.Watcher, error) {

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				switch event.Op {
				case fsnotify.Write:
					fileInfo, err := os.Lstat(event.Name)
					if err != nil {
						log.Println(err)
						break
					}
					if !p.isCompileUnit(fileInfo) {
						break
					}
					log.Println("Reloading ", fileInfo.Name())
					name, pluginLib, err := p.processPlugin(fileInfo)
					if err != nil {
						log.Println(err)
						break
					}
					for _, subscriber := range p.updateSubscribers {
						subscriber(name, pluginLib)
					}
				case fsnotify.Create:
					fileInfo, err := os.Lstat(event.Name)
					if err != nil {
						log.Println(err)
						break
					}
					if !p.isCompileUnit(fileInfo) {
						break
					}
					log.Println("Discovered ", fileInfo.Name())
					name, pluginLib, err := p.processPlugin(fileInfo)
					if err != nil {
						log.Println(err)
						break
					}
					for _, subscriber := range p.addSubscribers {
						subscriber(name, pluginLib)
					}
				case fsnotify.Rename:
					fallthrough
				case fsnotify.Remove:
					baseName := strings.TrimPrefix(event.Name, p.pluginDir+"/")
					baseName = strings.TrimSuffix(baseName, ".go")
					if pluginLib, exists := p.plugins[baseName]; exists {
						for _, subscriber := range p.removeSubscribers {
							subscriber(baseName, pluginLib)
						}
						delete(p.plugins, baseName)
					}
					log.Println("Removed ", baseName)
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(fileName)
	if err != nil {
		return nil, err
	}
	return watcher, nil
}

/*
scan will scan the whole plugin directory for .go files, compile them, load them and notify scan subscribers.
*/
func (p *Pluginator) scan() {

	files, err := ioutil.ReadDir(p.pluginDir)
	if err != nil {
		log.Println(err)
		return
	}
	for _, file := range files {
		if p.isCompileUnit(file) {
			log.Println("Discovered ", file.Name())
			_, _, err = p.processPlugin(file)
			if err != nil {
				log.Println(err)
				return
			}
		}

	}

	for _, scanSubscriber := range p.scanSubscribers {
		scanSubscriber(p.plugins)
	}
}

func (p *Pluginator) isCompileUnit(file os.FileInfo) bool {
	return !file.IsDir() && strings.HasSuffix(file.Name(), ".go")
}

func (p *Pluginator) processPlugin(file os.FileInfo) (string, *PluginContent, error) {

	var baseName string
	var pluginLib *plugin.Plugin

	baseName = strings.TrimSuffix(file.Name(), ".go")
	var err error
	pluginLib, err = p.compileAndLoad(baseName)
	if err != nil {
		return "", nil, err
	}
	code, err := ioutil.ReadFile(p.pluginDir + "/" + file.Name())
	if err != nil {
		return "", nil, err
	}
	pc := PluginContent{
		Lib:  pluginLib,
		Code: string(code),
	}
	p.plugins[baseName] = &pc
	return baseName, &pc, nil
}

func (p *Pluginator) compileAndLoad(baseName string) (*plugin.Plugin, error) {

	version, err := p.genVersionedName(baseName)
	if err != nil {
		return nil, err
	}
	address := fmt.Sprintf("%p", &p)
	pPath := "\"" + "-pluginpath=" + address + baseName + version + "\""
	command := exec.Command("go", "build", "-ldflags", pPath, "-buildmode=plugin", "-o", p.tempDir+"/"+baseName+"."+version+".so", p.pluginDir+"/"+baseName+".go")

	var stdErr bytes.Buffer
	command.Stderr = &stdErr
	_, err = command.Output()
	if err != nil {
		return nil, errors.New(stdErr.String())
	}
	pluginLib, err := plugin.Open(p.tempDir + "/" + baseName + "." + version + ".so")
	if err != nil {
		return nil, err
	}
	log.Println("Loaded ", baseName+"."+version+".so")
	return pluginLib, nil
}

func (p *Pluginator) genVersionedName(baseName string) (string, error) {

	files, err := ioutil.ReadDir(p.tempDir)
	if err != nil {
		log.Println(err)
		return "", err
	}
	latestVersion := baseName
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), baseName) && strings.HasSuffix(file.Name(), ".so") && file.Name() > latestVersion {
			latestVersion = file.Name()
		}
	}
	if latestVersion != baseName {
		fSplit := strings.Split(latestVersion, ".")
		ver := fSplit[1]
		verI, err := strconv.Atoi(ver)
		if err != nil {
			return "", err
		}
		verI++
		return fmt.Sprintf("%09d", verI), nil
	}
	return fmt.Sprintf("%09d", 0), nil
}

func (p *Pluginator) watchConsul() (*consulWatcher, error) {

	cw, err := newConsulWatcher(p.consulHost, p.consulPort, p.consulKeyPrefix)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			select {
			case event := <-p.consulWatcher.Events:
				switch event.Action {
				case consulAddAction:
					if !strings.HasSuffix(event.Key, ".go") {
						log.Println("Bad plugin name must end in .go: ", event.Key)
						break
					}
					p.materializeKV(event.Key, event.Value)
				case consulUpdateAction:
					p.materializeKV(event.Key, event.Value)
				case consulRemoveAction:
					p.unMaterializeK(event.Key)
				}
			}
		}
	}()
	return cw, nil
}

func (p *Pluginator) materializeKV(key, value string) {
	key = strings.TrimPrefix(key, p.consulKeyPrefix+".")
	if err := ioutil.WriteFile(p.pluginDir+"/"+key, []byte(value), os.ModePerm); err != nil {
		log.Println(err)
	}
}

func (p *Pluginator) unMaterializeK(key string) {
	key = strings.TrimPrefix(key, p.consulKeyPrefix+".")
	if err := os.Remove(p.pluginDir + "/" + key); err != nil {
		log.Println(err)
	}
}
