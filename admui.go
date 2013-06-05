package main

import (
	"encoding/json"
	"fmt"
	"log"
	"io"
	"net/http"
)

// SrvConfig defines the far-end server, and its command and payload ports
type SrvConfig struct {
	Host    string
	RPCPort string
	Count   uint64
	Repeat  bool
}

// Command controls the type of function that TCPClient should perform
type Command struct {
	Name string
	Cfg  SrvConfig
}

type BitRate uint64

// Returns bitrate in Mega-bits per second
func (b BitRate) Mbps() float32 {
	return float32(b) / float32(1000000)
}

// Returns bitrate in Mega-bytes per second
func (b BitRate) MBps() float32 {
	return float32(b) / float32(8*1000000)
}

// Returns bitrate in Kilo-bits per second
func (b BitRate) Kbps() float32 {
	return float32(b) / float32(1000)
}

// Returns bitrate in Kilo-bytes per second
func (b BitRate) KBps() float32 {
	return float32(b) / float32(8*1000)
}

func (b BitRate) String() string {
	return fmt.Sprintf("%d", b)
}

// Stats is type of measurement that TCPClient reports on its stats channel.
type Stats struct {
	Stat string
	Type string
	Rate BitRate
}

type JSONStats struct {
	Stat string
	Type string
	Rate float32
}

// CCmdHandler is the receiver type for handling TCPClient control request
type CCmdHandler struct {
	CmdCh chan Command
}

// CStatHandler is the reciever type for handling TCPClient stats requests
type CStatHandler struct {
	StatCh chan Stats
}

// This handler parses the form from the user and initiates a TCPClient
// measurement.
func (c *CCmdHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		raddr  string
		rport  int
		pktt   string
		tstt   string
		txsize int
		txmult string
		txcont string
	)
	params := map[string]interface{}{
		"raddr":  &raddr,
		"rport":  &rport,
		"pktt":   &pktt,
		"tstt":   &tstt,
		"txsize": &txsize,
		"txmult": &txmult,
		"txcont": &txcont,
	}
	Mult := map[string]uint64{
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
	}

	getformparams(r, params)
	trace.Printf("|CMD|%s|%s|\n", tstt, raddr)
	log.Println("CMD: ", raddr, rport, pktt, tstt, txsize, txmult, txcont != "")

	cmd := Command{
		Name: tstt,
		Cfg: SrvConfig{
			Host:    raddr,
			RPCPort: fmt.Sprint(rport),
			Count:   uint64(txsize) * Mult[txmult],
			Repeat:  txcont != "",
		},
	}
	c.CmdCh <- cmd
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(""))
}

// parse and store form parameters in the map that's passed in
func getformparams(r *http.Request, params map[string]interface{}) {
	for i, x := range params {
		if v := r.FormValue(i); v != "" {
			fmt.Sscan(v, x)
		}
	}
}

// This handler deals with GET requests for TCPClient measurement results.
// It returns the measurements since last snapshot in json format.
func (s *CStatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var jst JSONStats
	w.Header().Set("Content-Type", "text/plain")
	st, ok := <-s.StatCh
	if !ok {
		jst = JSONStats{Stat: "Error"}
	} else {
		jst = JSONStats{st.Stat, st.Type, st.Rate.Mbps()}
	}
	je := json.NewEncoder(w)
	je.Encode(jst)
}

// WebUI is an http server that provides an html UI to the user, annoucing itself at address
// that is passed in. It handles requests for starting and stopping of the load testing
// client and reporting of data.
func WebUI(addr string, cch chan Command, sch chan Stats) {
	cl := &CCmdHandler{cch}
	st := &CStatHandler{sch}
	http.Handle("/cmd", cl)
	http.Handle("/stats", st)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, htmlfile)

	})
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: " + err.Error())
	}
}

// unfortunately there isn't an easy way to include
// a file; so make the changes to index.html and then
// replace the contents of the htmlfile const with it.
const htmlfile = `
<!DOCTYPE html>
<html>
  <head>
    <script type='text/javascript' src='https://www.google.com/jsapi'></script>
    <script src="http://yui.yahooapis.com/3.8.0/build/yui/yui-min.js"></script>
    <script type='text/javascript'>
      google.load('visualization', '1', {packages:['corechart', 'gauge']});
	</script>
	<script type='text/javascript'>
    YUI().use("node", "io", "io-form", "event", "dump", "json-parse", "button", function(Y) {
        var that = this;
        var running = false;
        var gauge_options = {
            width: 200, height: 200,
            redFrom: 0, redTo: 10,
            yellowFrom: 10, yellowTo: 50,
            greenFrom: 50, greenTo: 100,
            minorTicks: 6,
        };
        var chart_options = {
            width: 600, height: 200,
            title : 'Megabits / Second',
            animation : { duration : 500 },
        };

        var upgauge = new google.visualization.Gauge(Y.one('#upgauge').getDOMNode());
        var dngauge = new google.visualization.Gauge(Y.one('#dngauge').getDOMNode());
        var upchart = new google.visualization.LineChart(Y.one('#upchart').getDOMNode());
        var dnchart = new google.visualization.LineChart(Y.one('#dnchart').getDOMNode());
        var dtable = new google.visualization.DataTable();
        var lUp = 0, lDown = 0;
        dtable.addColumn('timeofday', 'Time');
        dtable.addColumn('number', 'Bitrate');
        upchart.draw(dtable, chart_options);
        dnchart.draw(dtable, chart_options);
        upgauge.draw(google.visualization.arrayToDataTable([['Label', 'Value'], ['Upload', 0]]), gauge_options);
        dngauge.draw(google.visualization.arrayToDataTable([['Label', 'Value'], ['Download', 0]]), gauge_options);

        var onSuccess = function (id, o, args) {
            dtable = new google.visualization.DataTable();
            dtable.addColumn('timeofday', 'Time');
            dtable.addColumn('number', 'Bitrate');
            Y.one('#status_div').setHTML("<i>Starting...</i>");
            Y.all('#tstreqform input').setAttribute('disabled', 'disabled');
            Y.later(250, that, updateVisuals, false);  // give it time so that GET "/stats" doesn't fail right away
        };
        var onFailure = function (id, o, args) {
            Y.one('#start').removeAttribute('disabled');
            Y.all('#tstreqform input').removeAttribute('disabled');
            running = false;
            Y.one('#params_div').setStyle('opacity', '');
        }

        var enableForm = function () {
            Y.one('#start').removeAttribute('disabled');
            Y.all('#tstreqform input').removeAttribute('disabled');
            running = false;
            Y.one('#params_div').setStyle('opacity', '');
        };
        Y.one('#start').on('click', function(e) {
            if (running) return;
            Y.one('#start').setAttribute('disabled', 'disabled');
            running = true;
            Y.one('#params_div').setStyle('opacity', '0.4');
            e.halt(true);
            var cfg = {
                method : "POST",
                form : { id : Y.one('#tstreqform'), useDisabled : false },
                on : { success : onSuccess , failure : onFailure },
                context : Y,
                arguments : { success : '/cmd' },
            };
            Y.io("/cmd", cfg);
        });

        enableForm();
        /*
        Y.one('#stop').on('click', function(e) {
            var cfg = { method : "POST", data : "tstt=STOP" }
            Y.io("/cmd", cfg);
        });
        */

        var updateVisuals = function () {
            Y.io("/stats", {
                on : {
                    success : function (tx, r) {
                        var pr;
                        try {
                            pr = Y.JSON.parse(r.responseText);
                        } catch (e) {
                            alert("JSON Parse failed: ", r.responseText);
                            Y.one('#status_div').setHTML("<i>Error: "+e+"</i>");
                            enableForm();
                            return;
                        }
                        var msg = pr.Stat + " " + ((pr.Type=="UP")?"Upload":((pr.Type=="DOWN")?"Download":''));
                        Y.one('#status_div').setHTML("<i>"+msg+"</i>");
                        if (pr.Stat != "Running") {
                            enableForm();
                            return;
                        }
                        var foo = new Date();
                        var xtm = [foo.getHours(), foo.getMinutes(), foo.getSeconds(), foo.getMilliseconds()];
                        dtable.addRows([[xtm, pr.Rate]]);
                        if (pr.Type == "UP") {
                            upchart.draw(dtable, chart_options);
                            lUp = pr.Rate;
                            var dt = google.visualization.arrayToDataTable([
                                ['Label', 'Value'],
                                ['Upload', lUp ]
                                ]);
                            upgauge.draw(dt, gauge_options);
                        } else {
                            dnchart.draw(dtable, chart_options);
                            lDown = pr.Rate;
                            var dt = google.visualization.arrayToDataTable([
                                ['Label', 'Value'],
                                ['Download', lDown ]
                                ]);
                            dngauge.draw(dt, gauge_options);
                        }
                        Y.later(500, that, updateVisuals, false);
                    },
                    failure : function (tx, r) {
                        alert("No data");
                        enableForm();
                    },
                },
            });
        };
    });
    </script>
	<link rel="stylesheet" type="text/css" href="http://yui.yahooapis.com/3.8.0/build/cssfonts/cssfonts-min.css">
    <link rel="stylesheet" type="text/css" href="http://yui.yahooapis.com/3.8.0/build/cssgrids/grids-min.css">
  </head>
  <body>
   <div> <!-- class="yui3-cssfonts"> -->
    <h1>tcpmeter - TCP Speedometer</h1>
    <div class="yui3-skin-sam">
      <div class="yui3-g">
        <div id="params_div" class="yui3-u-1-4">
          <form id="tstreqform" method="post" action="/cmd">
			<fieldset>
			  <legend>Server Information</legend>
              <p>
              <label>Host:<input type=text name=raddr required></label><br />
              <label>RPC:<input type=number name=rport default="8001" placeholder="8001" required></label>
              </p>
			</fieldset>
            <p>
            <fieldset>
              <legend>Packet Type</legend>
              <p>
              <input type=radio name=pktt value="udp" disabled="disabled">UDP</input>
              <input type=radio name=pktt value="tcp" checked="checked">TCP</input>
              </p>
            </fieldset>
            </p>
            <p>
            <fieldset>
              <legend>Type of Measurement</legend>
              <p>
              <input type=radio name=tstt value="UP" checked="checked">Upload</input>
              <input type=radio name=tstt value="DOWN">Download</input>
              <input type=radio name=tstt value="RTT">Round Trip</input><br />
              <input type=checkbox name=txcont checked="">Continuous</input>
              </p>
            </fieldset>
            </p>
            <p>
            <fieldset>
              <legend>Size of Dataset</legend>
              <p>
              <label>Amount of data:<input type=number name=txsize></label><br />
              <input type=radio name=txmult value="KB">KB</input>
              <input type=radio name=txmult value="MB" checked="checked">MB</input>
              <input type=radio name=txmult value="GB">GB</input>
              </p>
            </fieldset>
            </p>
          </form>
          <p>
          <input id="start" type="button" value="Start" />
          <!-- <input id="stop" type="button" value="Abort" /> -->
          </p>
        </div>
		<div class="yui3-u-3-4">
            <div class="yui3-g">
                <div class="yui3-u-1-4" id='upgauge'></div>
                <div class="yui3-u-3-4" id='upchart'></div>
            </div>
            <div class="yui3-g">
                <div class="yui3-u-1-4" id='dngauge'></div>
                <div class="yui3-u-3-4" id='dnchart'></div>
            </div>
            <div id='status_div' style="text-align:center"><p><i>Stopped</i></p></div>
		</div>
      </div>
    </div>
   </div>
  </body>
</html>`
