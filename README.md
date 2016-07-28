livedev
=======

livedev is a development proxy server for golang that allows live reloading.  
It supports multiple server configuration.  
It uses the request's header "Host" field to determine which server the request should be routed to.  
If no match is found the request is routed the default server.

 
Compatible with: `go version go1.0.2+`

Features
========
* Cross-platform
* Unobstructive. No code change required
* Simple json configuration file
* Multiple server support
* Automated build service.
* Dependency change detection 
* Autorealod the page when assets (js, css, image, html...) change


Installation
============

`go get -u github.com/qrtz/livedev` 

Configuration
=============
livedev is controlled by a json configuration file:

* __port__: (int, default:"80") proxy port
* __GOROOT__: (string, optional) 
* __GOPATH__: (string, optional)
* __server__: ([]Server) A list of Server object with the following options:
    * __GOROOT__: (string, optional)  Server specific GOROOT for compiling with different go version
    * __GOPATH__: ([]string, optional) Server specific GOPATH.
    * __host__: (string) server hostname (must be unique)
    * __port__: (int, optional) server port  
    * __target__: (string, optional) Build target. The file that contains the main function.  
 if __target__ is not in the GOPATH, livedev will attempt to add it by guessing the workspace from the filename.  
 When __target__ is ommited, the build step is skipped.
    * __workingDir__: (string, optional) workingDir specifies the working directory of the server executable. If workingDir is empty, it defaults to the parent directory of the executable.  
    * __env__: (map, optional) A map of key value pairs to set as environment variables on the server.
    * __resources__: (optional) A list of resources such as template files. Any change to these files will cause the server to restart.
        * __ignore__: (string, optional) filename pattern to ignore. 
        * __paths__: ([]string) A list of files or directories to monitor
    * __assets__: (optional) A list of assets such as css, javascript, image files. Any change to these files will cause a page to reload.
        * __ignore__: (string, optional) filename pattern to ignore.
        * __paths__: ([]string) A list of files or directories to monitor
    * __bin__: (string, optional) server executable file. When absent, it default to /tmp/livedev[hostname]
    * __builder__: ([]string, optional) To use a builder other than the go build tool. The first element is the command and the rest its arguments
    * __startup__: ([]string, optional) server startup argument list
    * __default__: (bool, optinal) Specifies the default server.  
 Defaults to the first server in the list
    * __startupTimeout__: (int, default=5) Specifies the time (in seconds) limit  to wait for the server to complete the startup operation.

In the server configuration block, properties can be referred on using "${PROPERTY}" or "$PROPERTY" variable substutitions  
Along with the configuration properties, the process environment variables are also available.  

Usage
=====
```shell    
$ livedev -c config.json
```

### config.json 

    {
        "port":8080,
        "server":[
            {
                "host":"dev.service1.com",
                "port": 8081,
                "target":"/projects/src/serviceone/main.go",
                "workingDir": "/projects/src/serviceone"
                "resources":{
                    "ignore":"static*",
                    "paths":["${workingDir}/templates"]
                 },
                "startup": ["-host", "$host", "-port", "${port}"]
                "bin":"/projects/bin/serviceone"
            },
            {
                "host":"dev.service2.com",
                "env": {
                    "HOST": "${host}",
                    "PORT": "${port}"
                },
                "target":"/projects/src/servicetwo/main.go",
                "workingDir": "/projects/src/servicetwo"
                "resources":{
                    "ignore":"static*",
                    "paths":["${workingDir}/templates"]
                 },
                "bin":"/projects/bin/servicetwo"
            }
        ]
    }


```shell
# host file
127.0.0.1 dev.service1.com dev.service2.com
```
## dev.service1.com
URL: http://dev.service1.com:8080    
The request is forwarded to http://dev.serviceone.com:8081  
The server access "host and "port" from the command-line argument as specified in the "startup" property of the configuration

```go
packgage main

import (
    "flag"
    "net"
    "net/http"
) 

func main(){
    host := flag.String("host", "localhost", "host name")
    port := flag.String("port", "8081", "port")

    flag.Parse()

    addr := net.JoinHostPort(*host, *port)
    http.ListenAndServe(addr, handler)
}
```
## dev.service2.com
URL: http://dev.service2.com:8080  
The request is forwarded to http://dev.service2.com:`port`. Where `port` is a randomly generated port.  
The server gets access to "host and "port" from environment variables as specified in the "env" property of the configuration  

```go
packgage main

import (
    "net"
    "net/http"
    "os"
) 

func main(){
    host := os.Getenv("HOST")
    port := os.Getenv("PORT")
    addr := net.JoinHostPort(host, port)
    http.ListenAndServe(addr, handler)
}
```



