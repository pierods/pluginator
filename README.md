# Pluginator - a plugin manager for scripted Go

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![](https://godoc.org/github.com/pierods/pluginator?status.svg)](http://godoc.org/github.com/pierods/pluginator)
[![Go Report Card](https://goreportcard.com/badge/github.com/pierods/pluginator)](https://goreportcard.com/report/github.com/pierods/pluginator)
[![Build Status](https://travis-ci.org/pierods/pluginator.svg?branch=master)](https://travis-ci.org/pierods/pluginator)

Pluginator is a plugin manager/loader for plugins written in Go. Plugins can be dropped in source code form into a watched folder,
or added as subkeys to a consul key, as source code as well. Pluginator will pick them up, compile them, load the resulting libraries
and make them available to its clients.

Plugins can be edited in-place, added or removed. Pluginator will synchronize its internal registry with the watched folder (or consul
key) and notify its clients.

A higher level client of Pluginator can then decide what makes a valid plugin, and what can be in a plugin.

## Motivation
Scripting engines for Go are already available, like [otto](https://github.com/robertkrimen/otto), which executes javascript, or [go-lua](https://github.com/Shopify/go-lua), which executes lua.

However, there are three drawbacks to the scripting engine approach to plugins, which can be important or not for your project. The first is the loss of speed, which of course is not a problem for some applications.
The second is the full support for the target language, which limits the power of the scripting engine itself. Some scripting engines are very up-to-date with their target language,
but they inevitably fall behind.

The third drawback, maybe the most important, is the loss of expressivity. When a host language (Go in this case) is chosen for a domain problem, it's usually because it is 
the most expressive language for the specific problem at hand. Having to write plugins in another language causes a loss of that expressive power, and also, because of the 
learning curve of the target language, a further degradation in expressive power.

### Why Consul
Nowadays, it's very common to work with many instances of a program (think microservices). Having software that is not dependent on physical location of files is much more convenient.

## Limitations
Mainly three: a go toolchain must be installed on the host machine, Go >= 1.8 must be used to compile pluginator and for the go toolchain, and the target machine can only be linux.

## Installation
To compile pluginator, you need the consul Go client, fsnotify and Google UUID:
  
 ```bash
    go get github.com/hashicorp/consul/api
    go get github.com/google/uuid  # only for testing
    go get github.com/fsnotify/fsnotify
 
 ```
You can then build it:

```bash
    cd pluginator
    go build
```

You must also install a go toolchain on the host machine. Follow the instructions on Go's download page, [this one](https://golang.org/doc/install?download=go1.8.3.linux-amd64.tar.gz) for 1.8.3 for example

## Usage
You can instantiate Pluginator in file mode or consul mode:

```Go
    pluginator, err := NewPluginatorF("/a/chosen/plugin/directory")
    if err != nil {
        t.Fatal(err)
    }
```

```Go
    cw, err := newConsulWatcher("aconsulhost", 8500, "my.consul.key.for.plugins")
    if err != nil {
        t.Fatal(err)
    }
```

Your program can then subscribe to scan/add/modify/remove events:

```Go
    func ScanSubscriber(pluginNamesAndLibs map[string]*PluginContent) {
        // do something about plugins having been addded
    }
    
    func RemoveSubscriber(pluginName string, pluginLib *PluginContent) {
        // do something about a plugin having been removed
    }
    
    func UpdateSubscriber(pluginName string, pluginLib *PluginContent) {
        // do something about a plugin having been changed
    }
    
    func AddSubscriber(pluginName string, pluginLib *PluginContent) {
        // do something about a plugin having been added
    }
    
    pluginator.SubscribeScan(ScanSubscriber)
    pluginator.SubscribeRemove(RemoveSubscriber)
    pluginator.SubscribeAdd(AddSubscriber)
    pluginator.SubscribeUpdate(UpdateSubscribe)

```

You can then drop a go plugin in the plugin directory, or add it to consul (with the Go api or simply with an http client like curl):

```Go
    import "github.com/hashicorp/consul/api"
    
    config := api.DefaultConfig()
    	(*config).Address = "localhost:8500"
    
    	client, err := api.NewClient(config)
    	if err != nil {
    		...
    	}
    
    	kv := client.KV()
    	
        p := &api.KVPair{
            Key:   "my.plugin.key." + "myplugin.go"
            Value: []byte(myPlugin),
        }
    
        _, err = kv.Put(p, nil)
        if err != nil {
            ....
        }

```
Pluginator will notify its subscriber with a plugin's name, exported symbols and source code:

```Go
    type PluginContent struct {
        Lib  *plugin.Plugin
        Code string
    }
```

When you are done with pluginator, terminate it:

```Go
    pluginator.Terminate()
```

## Rules for plugins
A Pluginator plugin must:
 + be in package main
 + be in a filename ending in .go (or be a consul key with a name ending in .go)
 + can have a func main() stub for compiling locally before sending to Pluginator 
  
Here is an example plugin (more in the tests):

```Go
    package main
    
    func Add(x, y int) int {
        return x + y
    }
    
    func main() {
    }
```


