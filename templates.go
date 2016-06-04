package main

import (
	"fmt"
	"html/template"
)

var errTemplate = template.Must(template.New("error").Parse(fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en-US" dir="ltr">
	<head>
	<meta charset="UTF-8">
	<title>{{.Name}}</title>
	<link href="//fonts.googleapis.com/css?family=Open+Sans:400italic,400,300,700|Lora:400,700,400italic" rel="stylesheet" type="text/css" />
		<style type="text/css">
			html, body{
				padding: 5px;
				margin: 0;
				font: 14px/1.4 'Courier', monospace;
				color: red;
				background-color: #fafafa;
			}

			code span{
				white-space: pre;
			}
		</style>
	</head>
	<body>
		<h1 class="name">{{.Name}}</h1>
		<p class="message">{{.Message}}</p>
		<code>
		{{ range .Data}}
			{{ if .Link}}
				<a href="//{{.Link}}:{{.Line}}#L{{.Line}}">{{.Text}}</a>
			{{else}}
				<span>{{.Text}}</span>
			{{end}}
		{{end}}
		</code>
		%s
	</body>
</html>`, liveReloadHTML)))

var codeviewerTemplate = template.Must(template.New("codeviewer").Parse(`
<!DOCTYPE html>
<html lang="en-US" dir="ltr">
	<head>
	<meta charset="UTF-8">
	<title>{{.Title}}</title>
	<link href="//fonts.googleapis.com/css?family=Open+Sans:400italic,400,300,700|Lora:400,700,400italic" rel="stylesheet" type="text/css" />
		<style type="text/css">
			html, body{
				padding: 0;
				margin: 0;
				font: 14px/1.4 'Courier', monospace;
				color: #333;
				background-color: #fafafa;
				white-space: nowrap;
			}

			li,code, div{margin: 0;padding: 0;overflow:visible;}

			body{
				padding-bottom: 20px;
			}

			ol{
				counter-reset: i;
				list-style-type: none;
				padding: 0;
				margin: 0;
			}

			li:before{
				display:block;
				float: left;
				clear:left;
				counter-increment: i;
				content: counters(i, ",");
				padding: 0;
				margin: 0 0 0 -65px;
				width: 50px;
				text-align: right;
				color: #999;
				border-width: 0 1px 0 0;
				height: inherit;
				line-height: inherit;
				white-space: nowrap;
			}

			li{
				padding: 0 0 0 65px;
				margin: 0;
				display: block;
				float: left;
				width: 100%;
				background: transparent url("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQImWPYuXOnIgAGpwJNoQX4YAAAAABJRU5ErkJggg==") repeat-y 55px 0;
			}

			li code{
				display: block;
				float: left;
				white-space: pre;
			}


			li code.operator {
				color: #93A1A1;
			}

			li code.comment {
				color: #998;
			}

			li code.keyword {
				color: #AE81FF;
			}

			li code.define {
				color: #859900;
			}

			li code.string {
				color: #2AA198;
			}

			li code.number {
				color: #F5871F;
			}

			li code.funcall {
				color: #268BD2;
			}

			li:hover{
				background-color: #EBF2FC;
			}

			li:hover:before{
				font-weight: bold;
				color: #000;
			}

			li.err code, li.err:before{
				color: #dc322f;
			}
		</style>
	</head>
	<body>
		<div>
			{{with $src := .}}
			<ol>
			{{ range $src.Lines}}
				<li id="L{{.Num}}" {{if eq $src.ErrorLine .Num}}class="err"{{end}}>
					{{ range .Tokens}}
						<code class="{{.Kind}} {{.Name}}">{{.Text}}</code>
					{{end}}
				</li>
			{{end}}
			</ol>
			{{end}}
		</div>
	</body>
</html>`))
