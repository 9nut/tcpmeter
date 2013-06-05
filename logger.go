package main

import (
	"log"
)

// Continually log any stats, provide them on an output channel
// to downstream receivers (usually a CStatHandler)
func LogClient(si chan Stats, so chan Stats) {
	log.Println("LogClient started...")
	for {
		stats, ok := <-si
		if !ok {
			log.Fatal("receive failed")
		}
		if stats.Stat == "Running" {
			trace.Printf("|DATA|%s|%d|\n", stats.Type, stats.Rate)
		}
		select {
		case so <- stats:
		default:
		}
	}
}
