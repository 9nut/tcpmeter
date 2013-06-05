package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"time"
)

type TCPWorker interface {
	GetName() string
	Work(stop <-chan bool, stats chan<- uint64, nbytes uint64, addr string)
	GetRPC() string
}

// Type TCPSender implements TCPWorker interface for Upload speed test
// It contains the name of the server side RPC function to call to initiate testing
type TCPSender string

func (s TCPSender) GetName() string {
	return "UP"
}

func (s TCPSender) GetRPC() string {
	return string(s)
}

// TCPSender Work method uploads nbyte bytes to tcp address addr; if it receives anything on
// the stop channel sch, it exits. it periodically reports the number of bytes it
// transfered since the last report on cch
func (s TCPSender) Work(sch <-chan bool, cch chan<- uint64, nbytes uint64, addr string) {
	defer close(cch) // to signal the launcher we exited

	pktsize := uint64(8 * 1024)
	every := uint64(16 * pktsize)

	buf := make([]byte, pktsize)

	log.Println("About to dial ", addr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	incr := pktsize
	nw := 0

	for n := uint64(0); n < nbytes; n += uint64(nw) {
		if nbytes-n < pktsize {
			incr = nbytes - n
			buf = buf[:incr]
		}
		nw, err = conn.Write(buf)
		if err != nil {
			log.Println(err)
			return
		} else if nw != len(buf) {
			log.Println("Short write")
			return
		} else {
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		}
		select {
		case <-sch:
			return
		default:
			if (n+incr)%every == 0 {
				cch <- every
			}
		}
	}
	cch <- incr
}

// Type TCPReceiver implements TCPWorker interface for Download speed test
// It contains the name of the server side RPC function to call to initiate testing
type TCPReceiver string

func (r TCPReceiver) GetName() string {
	return "DOWN"
}

func (r TCPReceiver) GetRPC() string {
	return string(r)
}

// TCPReceiver Work method downloads nbyte bytes from tcp address addr; if it receives
// anything on the stop channel sch, it exits. it periodically reports the number of bytes
// it received since the last report on cch
func (r TCPReceiver) Work(sch <-chan bool, cch chan<- uint64, nbytes uint64, addr string) {
	defer close(cch) // to signal the launcher we exited

	pktsize := uint64(8 * 1024)
	every := uint64(16 * pktsize)

	buf := make([]byte, pktsize)

	log.Println("About to dial ", addr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	nw := 0
	chkpt := uint64(0)

L:
	for n := uint64(0); n < nbytes; n += uint64(nw) {
		nw, err = conn.Read(buf)
		if err != nil {
			log.Println(err)
			return
		} else {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		}
		chkpt += uint64(nw)
		select {
		case <-sch:
			break L
		default:
			if chkpt >= every {
				cch <- chkpt
				chkpt = 0
			}
		}
	}
	cch <- chkpt
}

func Dispatch(ch chan<- Stats, cfg SrvConfig, worker TCPWorker) error {
	name := worker.GetName()

	log.Println("Measuring ", name, " speed...")
	client, err := rpc.Dial("tcp", cfg.Host+":"+cfg.RPCPort)
	if err != nil {
		log.Println(err)
		return err
	}
	defer client.Close()

	Done := make(chan bool)
	Res := make(chan uint64)
	samples := make([]BitRate, 1, 21)
	var (
		rep      bool
		addr     string
		tcnt     uint64
		lcnt     uint64
		srvtotal uint64
		br       BitRate
	)

	err = client.Call("TCPPerf.TCPStart", 0, &addr)
	if err != nil {
		log.Println(err)
		return err
	}

	log.Println("Payload address: ", addr)

	rpcname := worker.GetRPC()
	log.Println("Calling ", rpcname, " ...")
	aRcv := client.Go(rpcname, cfg.Count, &srvtotal, nil)

	go worker.Work(Done, Res, cfg.Count, cfg.Host+":"+addr)

	log.Println("Entering wait loop")
	t0 := time.Now()
	t1 := t0
	bps := func(n uint64, t0, t1 time.Time) BitRate {
		// n*8 bits
		return BitRate(n * uint64(8e9) / uint64(t1.Sub(t0).Nanoseconds()))
	}
	avg := func(s []BitRate) BitRate {
		t := uint64(0)
		for _, x := range s {
			t += uint64(x)
		}
		return BitRate(t / uint64(len(s)))
	}
	addsamp := func() {
		tn := time.Now()
		xr := bps(lcnt, t1, tn)
		lcnt = uint64(0)
		t1 = tn
		samples = append(samples, xr)
		if len(samples) > 20 {
			samples = samples[len(samples)-20:]
		}
	}
	timer := time.Tick(500 * time.Millisecond)

L1:
	for {
		select {
		case <-timer:
			addsamp()
			br = avg(samples)
			// log.Println("Bitrate: ", br.Mbps(), " Mbps, samples:", len(samples))
			if tcnt >= cfg.Count {
				Done <- true
				break L1
			}
			select {
			case ch <- Stats{"Running", name, br}:
			default:
			}
		case count, ok := <-Res:
			if ok {
				tcnt += count
				lcnt += count
			} else {
				addsamp()
				br = avg(samples)
				// log.Println("Bitrate: ", br.Mbps(), " Mbps")
				break L1
			}
		}
	}

	ch <- Stats{"Running", name, br}

	<-aRcv.Done
	br = bps(tcnt, t0, time.Now())
	log.Println("My count: ", cfg.Count, " Server count: ", srvtotal, " Average: ", br.Mbps(), "Mbps")

	err = client.Call("TCPPerf.TCPStop", 0, &rep)
	if err != nil {
		log.Println(err)
	}
	return err

}

// TCPClient initiates Upload, Download or RTT measurements, based on
// the instructions sent to it over the Command chan and reports back
// its results over Stats chan.
func TCPClient(cch <-chan Command, sch chan<- Stats) {
	log.Println("TCPClient started")

	ops := map[string]TCPWorker{
		"UP":   TCPSender("TCPPerf.TCPRcv"),
		"DOWN": TCPReceiver("TCPPerf.TCPSnd"),
	}

	timer := time.Tick(1 * time.Second)
L:
	for {
		select {
		case c, ok := (<-cch):
			if !ok {
				close(sch)
				break L
			}
			log.Println("Command: ", c.Name)
			worker, found := ops[c.Name]
			if found {
				err := Dispatch(sch, c.Cfg, worker)
				if err != nil {
					log.Println(err)
				}
			} else if c.Name != "STOP" {
				log.Println("Unsupported command:", c.Name)
				select {
				case sch <- Stats{Stat: "Error", Type: "Illegal command:" + c.Name}:
				default:
				}
			}
		case <-timer:
			select {
			case sch <- Stats{Stat: "Stopped"}:
			default:
			}
		}
	}
}

func ClientMain(haddr string) {
	cch := make(chan Command)
	sch := make(chan Stats, 10)
	lch := make(chan Stats)
	go TCPClient(cch, sch)
	go LogClient(sch, lch)
	fmt.Printf("Open http://localhost%s in a browser\n", haddr)
	WebUI(haddr, cch, lch)
}
