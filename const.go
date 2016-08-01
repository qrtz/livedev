package main

const (
	envGopath = "GOPATH"
	envGoroot = "GOROOT"

	liveReloadProtocol = "livedev"
	liveReloadHTML     = `
	<script type="text/javascript">
	!function (w, c) {
		try{
			(new WebSocket('ws://' + w.location.host + ':%d/', 'livedev')).onclose=function(){w.location.reload()}
		}catch(ex){c.log('Livedev: ', err)}
	}(window, window.console||{log:function(){}})
    </script>
	`
)
