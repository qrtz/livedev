livedev
=======

livedev is a development proxy server for golang that allows live reloading.  
It supports multiple server configuration.  
It uses the request's header "Host" field to determine which server the request should be routed to.  
If no match is found the request is routed the default server.

 
It has been tested with: `go version go1.0.2`

Features
========
* Cross-platform
* Unobstructive. No code change required
* Simple json configuration file
* Multiple server support
* Automated build service.


Installation
============

`go get github.com/qrtz/livedev` 

Configuration
=============
livedev is controlled be a json configuration file:

* __port__: (number, default:"80") proxy port
* __GOROOT__
* __GOPATH__
* __server__: ([]Server) A list of Server object with the following options:
 * __host__: (string, default:'localhost') server hostname (must be unique)
 * __port__: (int, default: dynamically generated) server port  
 When omitted, the server must accept `addr=<hostname:port>` argument
 * __target__: (string, optional) 
 * __source__: ([]string, optional) A list of source directories to watch
 * __skip__: (string, optional) skip filename pattern
 * __bin__: (string, optional) server executable file
 * __startup__: ([]string, optional) server startup argument list
 * __default__: (bool, optinal) use as default server  
 The first server in the list used as default if no default is set
 
### Example:

    `{
        "port":80,
        "server":[
            {
                "skip":"static",
                "host":"example.com",
                "port": 8080,
                "target":"/projects/example/src/main.go"
                "source":"/projects/expemple"
                "bin":"/projects/example/bin/example"
                "startup":['-res', '/path/to/resource/directory'],
                "default":true
            }
        ]
    }`


Usage
=====

    $ livedev -c config.json
    
Assuming you used the above configuration and added `example.com` to your host file,
point your browser to [http://example.com](http://example.com)  
livedev will start your app, compiling it if necessary and foward the request to
it on the configured port.

You can have multiple app running with different hostname and port number



