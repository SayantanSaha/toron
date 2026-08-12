package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var startPort int
	var count int

	flag.IntVar(&startPort, "port", 9001, "Base port to start dummy services from (e.g. 9001)")
	flag.IntVar(&count, "count", 10, "Number of dummy services to run")
	flag.Parse()

	log.Printf("[DUMMY-SERVICES] Starting %d dummy web services on ports %d through %d...", count, startPort, startPort+count-1)

	cluster := NewCluster(startPort, count)
	cluster.StartAll()

	for _, srv := range cluster.Servers {
		log.Printf(" -> [%s] Listening on http://localhost:%d", srv.Config.Name, srv.Config.Port)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("[DUMMY-SERVICES] Stopping all dummy web services...")
	cluster.StopAll()
	log.Println("[DUMMY-SERVICES] All dummy services stopped cleanly.")
}
