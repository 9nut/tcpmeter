package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"time"
)

var SrvAddr *net.TCPAddr

type NullFile struct{}
type ZeroFile struct{}

func (d *NullFile) Write(p []byte) (int, error) {
	return len(p), nil
}

func (d *ZeroFile) Read(p []byte) (int, error) {
	return len(p), nil
}

// TCPPerf is the receiver type for TCP Performance RPC methods
type TCPPerf struct {
	LData   *net.TCPListener // payload data listener
	DevNull *NullFile
	DevZero *ZeroFile
}

// TCPStart method prepares the tcp link that will be used for tcp performance testing
func (p *TCPPerf) TCPStart(_ int, r *string) error {
	log.Println("TCPStart called")
	if p.LData != nil {
		p.LData.Close()
	}

	var err error
	p.DevNull = &NullFile{}
	p.DevZero = &ZeroFile{}

	addr := "" // use any available port for payload
	if SrvAddr.IP != nil {
		addr = SrvAddr.IP.String() + ":0"
	} else {
		addr = "localhost:0"
	}

	DAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Println("ResolveTCPAddr: ", err)
		return err
	}

	p.LData, err = net.ListenTCP("tcp", DAddr)
	if err != nil {
		log.Println("ListenTCP: ", err)
		return err
	}

	DAddr, ok := p.LData.Addr().(*net.TCPAddr)
	if ok {
		*r = fmt.Sprint(DAddr.Port)
	}

	log.Println("Payload port: ", *r)
	return nil
}

// TCPStop method tears down the tcp link that was used for tcp performance testing
func (p *TCPPerf) TCPStop(_ int, r *bool) error {
	*r = false
	log.Println("TCPStop called")
	p.LData.Close()
	p.LData = nil
	*r = true
	return nil
}

// accept with deadline will do a timed accept of the payload tcp, returning a TCPConn
// when possible
func (p *TCPPerf) timedaccept() (conn *net.TCPConn, err error) {
	log.Println("timedaccept called")
	if p.LData == nil {
		err = errors.New("No Payload TCP Listener")
		log.Println(err)
		return nil, err
	}

	stop := make(chan bool)
	go func(stop chan bool) {
		select {
		case <-time.After(5 * time.Second):
			log.Println("Timeout")
			p.LData.Close()
		case <-stop:
		}
	}(stop)
	conn, err = p.LData.AcceptTCP()
	if err != nil {
		log.Println("AcceptTCP", err)
	}
	stop <- true // there will be a race
	return
}

// TCPRcv method tries to receive the number of bytes given by the first parameter
// on a TCP host/port specified in the TCPPerf reciever.  If successful, it will store
// the number of bytes it actually received, at the location given by the second parameter.
func (p *TCPPerf) TCPRcv(n uint64, r *uint64) error {
	*r = 0
	log.Println("TCPRcv called")
	conn, err := p.timedaccept()
	if err != nil {
		log.Println("timedaccept", err)
		return err
	}
	defer conn.Close()

	ncpy, err := io.CopyN(p.DevNull, conn, int64(n))
	if err != nil {
		log.Println("CopyN error: ", err)
		return err
	}
	*r = uint64(ncpy)
	return nil
}

// TCPSnd method tries to send the number of bytes given by the first parameter
// on a TCP host/port specified in the TCPPerf reciever. It will store
// the number of bytes it actually sent, at the location given by the second parameter.
func (p *TCPPerf) TCPSnd(n uint64, r *uint64) error {
	*r = 0
	log.Println("TCPSnd called")
	conn, err := p.timedaccept()
	if err != nil {
		log.Println("timedaccept", err)
		return err
	}
	defer conn.Close()

	ncpy, err := io.CopyN(conn, p.DevZero, int64(n))
	if err != nil {
		log.Println("CopyN error: ", err)
		return err
	}
	*r = uint64(ncpy)
	return nil
}

// TCPCpy method listens on a TCP host/port specified in the TCPPerf receiver and
// once established, it copies everything it recieves back to the sender.
func (p *TCPPerf) TCPCpy(_ uint64, r *uint64) error {
	*r = 0
	log.Println("TCPCpy called")
	conn, err := p.timedaccept()
	if err != nil {
		log.Println("timedaccept", err)
		return err
	}
	defer conn.Close()

	ncpy, err := io.Copy(conn, conn)
	if err != nil {
		log.Println("Copy error: ", err)
		return err
	}
	*r = uint64(ncpy)
	return nil
}

// TCPServer listens and handles RPC calls from clients.
func TCPServer(raddr string) {
	var err error

	perf := new(TCPPerf)
	rpc.Register(perf)

	SrvAddr, err = net.ResolveTCPAddr("tcp", raddr)
	if err != nil {
		log.Fatal(err)
	}

	l, err := net.ListenTCP("tcp", SrvAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Starting server")
	rpc.Accept(l)
}
