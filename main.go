package main

import (
	"flag"
	"log"
	"os"
	"runtime/pprof"
)

var trace *log.Logger

func main() {
	var cf, sf bool
	var haddr, raddr string
	var fname, pname string
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	cmdline.Usage = func() {
		log.Printf("usage: %s (-c|-s) [-r [host:]port] [-h [host:]port] [-l logfile]\n", os.Args[0])
	}
	cmdline.BoolVar(&cf, "c", false, "client mode")
	cmdline.BoolVar(&sf, "s", false, "server mode")
	cmdline.StringVar(&raddr, "r", ":8001", "RPC address")
	cmdline.StringVar(&haddr, "h", ":8080", "Admin WebUI")
	cmdline.StringVar(&fname, "l", "/tmp/tcpmeter.log", "Log file name")
	cmdline.StringVar(&pname, "p", "", "CPU profile file")

	cmdline.Parse(os.Args[1:])

	if pname != "" {
		f, err := os.Create(pname)
		if err != nil {
			log.Fatal(err)
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}
	if cf == sf {
		cmdline.Usage()
		log.Fatalln("either -s or -c must be specified")
	}

	logfile, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0660)
	if err != nil {
		log.Fatal("OpenFile failed", err)
	}
	defer logfile.Close()
	trace = log.New(logfile, "", log.LstdFlags)
	log.SetFlags(log.Flags() | log.Llongfile)

	if cf {
		ClientMain(haddr)
	} else {
		TCPServer(raddr)
	}
}
