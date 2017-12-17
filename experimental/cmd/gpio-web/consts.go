// Copyright 2017 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"encoding/base64"
	"html/template"
)

var favicon []byte

var rootTpl *template.Template

func init() {
	var err error
	if favicon, err = base64.StdEncoding.DecodeString(faviconB64); err != nil {
		panic(err)
	}
	if rootTpl, err = template.New("root").Parse(rootRaw); err != nil {
		panic(err)
	}
}

var rootStyle = `
h1, h2, h3, h4, h5, h6 {
	margin-bottom: 0.2em;
	margin-top: 0.2em;
}
.col {
	/*-moz-column-count: 2;
	-webkit-column-count: 2;
	column-count: 2;*/
	padding-left: 5px;
	padding-right: 5px;
}
.pins {
	float: right;
}
canvas {
	border: 1px solid #888;
	display: block;
	margin: auto;
	margin-top: 10px;
	padding: 0;
}
.err {
	background: #F44;
	border: 1px solid #888;
	border-radius: 10px;
	padding: 10px;
	display: none;
}
`

var rootJs = `
// Drawing context.
var ctx;
// Error box.
var err;
// GPIOs elements.
var gpios;
var colors = ['green', 'blue', 'red', 'orange', 'yellow', 'purple', 'white', 'brown'];
var graphPins = [];
var timerGPIOs;

window.onload = function() {
	err = document.getElementById('err');
	var graph = document.getElementById('graph');
	ctx = graph.getContext('2d');
	gpios = document.getElementById('gpios');
	refreshGPIOs();
	drawGraphBackground();
	// TODO(maruel): Don't refresh if one is already on-going.
	timerGPIOs = setInterval(refreshGPIOs, 1000);
};

function refreshGraph() {
	// TODO(maruel): Disallow more than 8 pins.
	graphPins = [];
	var elems = document.getElementsByClassName('GPIOCB');
	for (var i = 0; i < elems.length; i++) {
		var e = elems[i];
		if (e.checked) {
		  graphPins.push(e.id);
		}
	}
	if (graphPins.length == 0) {
		drawGraphBackground();
		return;
	}
	graphPins.sort();
  var r = new XMLHttpRequest();
	var res = '10ms';
	var dur = '200ms';
  r.open('get', '/read?res=' + res + '&dur=' + dur + '&pins=' + graphPins.join(','), true);
	r.responseType = 'arraybuffer';
  r.onreadystatechange = function () {
    if (r.readyState === XMLHttpRequest.DONE) {
      if (r.status === 200) {
				err.style.display = 'none';
				drawGraphData(new Uint8Array(r.response));
				refreshGraph();
      } else if (r.status === 400) {
				var t = new TextDecoder('utf-8');
        err.innerText = t.decode(r.response);
				err.style.display = 'block';
      }
    }
  };
  r.send();
}

function refreshGPIOs() {
  var r = new XMLHttpRequest();
  r.open('get', '/all', true);
  r.onreadystatechange = function () {
    if (r.readyState === XMLHttpRequest.DONE) {
      if (r.status === 200) {
				updateGPIOsList(JSON.parse(r.responseText));
      } else if (r.status === 400) {
        err.innerText = r.responseText;
				err.style.visibility = 'visible';
      }
    }
  };
  r.send();
}

function drawGraphBackground() {
	ctx.fillStyle = 'rgb(0, 0, 0)';
	ctx.lineWidth = 1;
	ctx.strokeStyle = 'gray';
	ctx.fillRect(0, 0, graph.width, graph.height);
	for (var i = 10; i < graph.width; i += 20) {
		ctx.beginPath();
		ctx.moveTo(i, 0);
		ctx.lineTo(i, graph.height);
		ctx.stroke();
	}
	for (var i = 10; i < graph.height; i += 20) {
		ctx.beginPath();
		ctx.moveTo(0, i);
		ctx.lineTo(graph.height, i);
		ctx.stroke();
	}
}

// data is Uint8Array
function drawGraphData(data) {
	drawGraphBackground();
	ctx.fillStyle = 'rgb(0, 0, 0)';
	ctx.lineWidth = 3;
	for (var p = 0; p < graphPins.length; p++) {
		ctx.strokeStyle = colors[p];
		var base = 10 + 40*p;
		var mask = (1<<p);
		ctx.globalAlpha = 0.3;
		ctx.beginPath();
		ctx.moveTo(0, base);
		ctx.lineTo(graph.width, base);
		ctx.stroke();
		ctx.beginPath();
		ctx.moveTo(0, base+10);
		ctx.lineTo(graph.width, base+10);
		ctx.stroke();
		ctx.globalAlpha = 1;

		ctx.beginPath();
		ctx.moveTo(0, base + 10 * ((data[0] & mask) >> p));
		for (var i = 0; i < data.byteLength; i++) {
			ctx.lineTo(i * graph.width / data.byteLength, base + 10 * ((data[i] & mask) >> p));
			ctx.lineTo((i+1) * graph.width / data.byteLength, base + 10 * ((data[i] & mask) >> p));
		}
		ctx.stroke();
	}
}

// GPIO with {name: value}
function updateGPIOsList(data) {
	// TODO(maruel): Make it a table instead?
	var keys = Object.keys(data);
	keys.sort(naturalSort);
	for (var i = 0; i < keys.length; i++) {
		var key = keys[i];
		var v = data[key];
		var e = document.getElementById(key + '_label');
		if (!e) {
			var x = document.createElement('span');
			x.innerHTML = '' +
				'<input id="' + key + '" class="GPIOCB" type="checkbox" onchange="refreshGraph()">' +
				'<label id="' + key + '_label" for="' + key + '"></label><br>';
			gpios.appendChild(x);
			e = document.getElementById(key + '_label');
		}
		e.innerText = key + ' ' + v;
	}
}

/*
 * Natural Sort algorithm for Javascript - Version 0.8.1 - Released under MIT license
 * Author: Jim Palmer (based on chunking idea from Dave Koelle)
 * https://github.com/overset/javascript-natural-sort
 */
function naturalSort(a, b) {
	var re = /(^([+\-]?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?(?=\D|\s|$))|^0x[\da-fA-F]+$|\d+)/g,
		sre = /^\s+|\s+$/g,   // trim pre-post whitespace
		snre = /\s+/g,        // normalize all whitespace to single ' ' character
		dre = /(^([\w ]+,?[\w ]+)?[\w ]+,?[\w ]+\d+:\d+(:\d+)?[\w ]?|^\d{1,4}[\/\-]\d{1,4}[\/\-]\d{1,4}|^\w+, \w+ \d+, \d{4})/,
		hre = /^0x[0-9a-f]+$/i,
		ore = /^0/,
		i = function(s) {
				return (naturalSort.insensitive && ('' + s).toLowerCase() || '' + s).replace(sre, '');
		},
		// convert all to strings strip whitespace
		x = i(a),
		y = i(b),
		// chunk/tokenize
		xN = x.replace(re, '\0$1\0').replace(/\0$/,'').replace(/^\0/,'').split('\0'),
		yN = y.replace(re, '\0$1\0').replace(/\0$/,'').replace(/^\0/,'').split('\0'),
		// numeric, hex or date detection
		xD = parseInt(x.match(hre), 16) || (xN.length !== 1 && Date.parse(x)),
		yD = parseInt(y.match(hre), 16) || xD && y.match(dre) && Date.parse(y) || null,
		normChunk = function(s, l) {
				// normalize spaces; find floats not starting with '0', string or 0 if not defined (Clint Priest)
				return (!s.match(ore) || l == 1) && parseFloat(s) || s.replace(snre, ' ').replace(sre, '') || 0;
		},
		oFxNcL, oFyNcL;
	// first try and sort Hex codes or Dates
	if (yD) {
		if (xD < yD) { return -1; }
		else if (xD > yD) { return 1; }
	}
	// natural sorting through split numeric strings and default strings
	for(var cLoc = 0, xNl = xN.length, yNl = yN.length, numS = Math.max(xNl, yNl); cLoc < numS; cLoc++) {
		oFxNcL = normChunk(xN[cLoc] || '', xNl);
		oFyNcL = normChunk(yN[cLoc] || '', yNl);
		// handle numeric vs string comparison - number < string - (Kyle Adams)
		if (isNaN(oFxNcL) !== isNaN(oFyNcL)) {
			return isNaN(oFxNcL) ? 1 : -1;
		}
		// if unicode use locale comparison
		if (/[^\x00-\x80]/.test(oFxNcL + oFyNcL) && oFxNcL.localeCompare) {
			var comp = oFxNcL.localeCompare(oFyNcL);
			return comp / Math.abs(comp);
		}
		if (oFxNcL < oFyNcL) { return -1; }
		else if (oFxNcL > oFyNcL) { return 1; }
	}
}
`

var rootRaw = `<!DOCTYPE html>
<meta charset="utf-8" /> 
<title>{{.Hostname}}</title>
<style>` + rootStyle + `
</style>
<script>` + rootJs + `
</script>
<div class="err" id="err"></div>
<div class="row">
	<div class="col pins">
		<form>
			<fieldset id="gpios">
				<legend>GPIOs</legend>
				{{/*
				{{range $i, $e := .Bits}}
					<input id="gpio{{$i}}" type="checkbox" onchange="refreshGraph()">
					<label for="gpio{{$i}}">GPIO{{$i}}</label>
					<br>
				{{end}}
				*/}}
			</fieldset>
		</form>
	</div>
	<div class="col">
		<fieldset>
		<legend>State</legend>
		<h2>Loaded</h2>
		{{range .State.Loaded}}
		- {{.}}<br>
		{{end}}
		<h2>Skipped</h2>
		{{range .State.Skipped}}
		- {{.}}<br>
		{{end}}
		{{if .State.Failed}}
			<h2>Failed</h2>
			{{range .State.Failed}}
			- {{.}}<br>
			{{end}}
		{{end}}
		</fieldset>
	</div>
	<canvas id="graph" width="450" height="450"></canvas>
</div>
`

// Created with:
// python -c "import base64;a=base64.b64encode(open('gpio-web.png','rb').read()); print '\n'.join(a[i:i+70] for i in range(0,len(a),70))"
const faviconB64 = "" +
	"iVBORw0KGgoAAAANSUhEUgAAAIAAAACACAYAAADDPmHLAAAHaElEQVR42u2dzWtUVxjGfz" +
	"mZYUiY0SRetSKmJVHwgwhhNtKNu9Z0091oNWIVuyhU1FKE5A9IQApWXOpGHK3OWtB2587N" +
	"bcDBDyoJNFJEc9XEDNFhkkkXZ8DOR+Yryczcc97fcmDm3nme577n3HPOvaeNOonH48uFnw" +
	"0PD7fRIMod34vGOqGtF9QAtA9C+14I9kFwKwQiEAxBQIHKfTMLLGYhk4bFeci8gswULD2B" +
	"pQnIJmF52nETC374/7UQwCC86OlHsGEfhFRt32wH2hWEOoAOYAswAHyb//s/Z+H9Y/h497" +
	"4hmgX8aXRsV2kDNg+s75FDKneMASQAjTb98G4IXYCe49ARaM1zPJeBtzcgfdFx7zyTAKz+" +
	"Sg9D8Ax0j0Ik3PpydgRg+0ngpBf9KQXvxiBzxXETKQlAjSUeOi/DtiH/FtdIGCJjwJgX/f" +
	"4eLJx13MRzCUBZ448MQvj6+rfljWbbEDDkRU8nIXXCcW9PSACK2vdIwjzjS3VSN/+lgzAf" +
	"a4V+QqC5xse6oPOWv0t93UF4mmsajjpuYrZZZ6KaZ/6xEeh/Z5/5hU1D/zuthSUVQJf7ng" +
	"fQtQUhR++YF/3xHLw92OhmQTXW/OEx6Hsq5peiawv0PdUaGVYBvGjMgY0PwekXoyuxY8SL" +
	"/hCDuQOOm/B8XwG86HdfQd+MmF8LTj/0zWjt1pe2UrNKgj0okUACIEgABAmAYCV1L2Eq1X" +
	"k8dOlxw078/vl92Hv8uTf3z3+5qfDTepaESQXwJRs3SRMgSAAECYDQjAA0YohSaNEA6Imd" +
	"z/8Q6VoP7c26V4CND1dx9yis793Bw3UNgJ6rllm91sXpr3U9gare/MO7YceIiNzq7BjRXq" +
	"15Beh5IOL6heq9UtVd/cdGZBmXn+jaUu1CU1XZ/FgX9I6JqH6jd0x7t+oK0HlLxPQrlb1T" +
	"lTt+Nq/b9zvbhip1CCtUgEhCRPQ75T1UK1/9RwbNf1bPBjYPaC9rrgDh6yKeKazspVqh57" +
	"9Lrn7TqkBsVw0VoPOyiGbcHcHlqgKgX8siPX8z7whi4SoqQPCMiGUqxd6WCED3qAhlKsXe" +
	"qvzyf3i3P97GJdQ5JhAuHBgqqAChCyKS6eR7XBCAnuMikOnke6zy7/07AiKQ6XQE/j8mUP" +
	"f7AYZ/OdZyfy3+682GHcuU/y/PBViOBEACIEgABGvJbbFy+lGts3/Ov8XLBL3tjRtElOOv" +
	"9vgzSce9tj9XATbsk2vBNrTnSm+wFJKmwDpCyovGOpXeXUuwtAfQq/TWaoKl9wADSu+rJ9" +
	"hJ+6DSmyoKlgZgr9I7agp2EuxTejtVwdIAbFV6L13BTgIRpTdSFiytACGld9EWLK0ASsl8" +
	"kNXjAOK+RICsqGAtWRQsSgKsZTGrIJMWIWwlk1awOC9CWFsB5hVkXokQ1laAVwoyUyKEtQ" +
	"GYUrD0RISwlaUnCpYmRAhrAzChIJsUIawdB0gqWJ4WIWxleVo5bmIB0jIYZB3prOMmFnJz" +
	"Ae8fiyC2oT3PBeDjXRHENrTnbXy2vLxWPynP5/vv/8t0sOVIACQAggRAsJa8HSC96LlMtW" +
	"8Ku3+++InyQ5cadzcpx6/3+B8WHfe34AoV4O0NuSZMJ9/jggCkL4pAppPvcV4AHPfOM5hP" +
	"iUimMp/SHpftBL6TPQKNpdjbEgHIXBGhTKXY26IAOG4iBS/viVim8fKe9raqcYCFsyKYaZ" +
	"T2tGQAHDfxHGZkpZAxzCS1p1UGQJM6IcKZwsperhgAx709IVXAlKv/9kTNAcjdN8ZEQN/f" +
	"+5f1sGwA9KCB3BH4u+efP/BTYwUAWDgqQvq251/Ru4oBcNzELEzLXoK+Y3pUe7fKAOgQ3B" +
	"yH2dciql+Yfa09q0wNC0LeHhRh/UL1XlUdAN2ZeDEu4rY6L8YrdfzqrADguPFR8CZF5FbF" +
	"m9QeVU8dawLnDsCyaN2SzB2o9Rs1B8BxEx7887WI3Xpob9Y5APpAv/8pcpuBLAuXAAgSAE" +
	"ECIPiqt/+mRQOwdicmlNPY+2Ktfq0tHo/LTb00AYIEQJAACBIAwTLa6v1iqc7j8PBwyd/z" +
	"ojEHNj4Ep3+tTtzs9wN4kzB3oNzYfi36N70COG7Cc9yrO2U9QTW8GHfcqzvrmdhp+SZAz1" +
	"VP7ZHlZaWYfQ1Te2qdz18tgUb/zdxqla1e9NgI9Mqj6IBewHmzKdWxaZ1A/Ycnu+1+7uDl" +
	"PZjsbpb5TakABX2DWeAbL3p4N0QSsHnADuNnkjAfq2XtnpEBKGgW9nvRI4MQvm5uEGaSkD" +
	"pR7lk9KwPwKQi3J3QQYrug8zJsGzKn1C+cXekRbQlAcdPwXDcNsTAEz0D3KETC/jJ9PqXf" +
	"yZO5UurNHBKA6oKQAsaBcd1PCF2AnuPVvsyy8XxY1O/hS19shfbd9wEo0U84BZzSTcS+v1" +
	"vvHD+9gdMv+HIuYOW2dCa5vtvfpLP6GOaMaAYwCMe9th/Ai8Y6oa0X1AC0D0L7Xgj2QXAr" +
	"BCIQDEFAfcp/Fr2Jdiatt9LNvNIbai490dvqZZOwPK33V9LEOTRigmb/AfdRab199hvGAA" +
	"AAAElFTkSuQmCC"
