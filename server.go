package main

import (
	"net/http"
	"sync"
	"time"
	"log"
	"flag"

	"github.com/gorilla/mux"
	"github.com/go-redis/redis"
	"github.com/vharitonsky/iniflags"

	"ADFPserver/client"
	"ADFPserver/routes"
	"ADFPserver/utility"

)

var (
	addr = "192.168.11.146:8087"
	redisClient *redis.Client
	clients map[string]*client.Client // List of active client connections
)

var (

	writeWait = flag.Int("writeWait", 10, "writeWait")
	// Time allowed to write a message to the peer
	// writeWait = 10

	// Time allowed to read the next pong message from the peer
	pongWait = flag.Int("pongWait", 30, "pongWait")
	// pongWait = 30

	// Maximum message size allowed from peer
	maxMessageSize = flag.Int64("maxMessageSize", 1024, "maxMessageSize")
	// maxMessageSize = 1024

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (*pongWait * 9) / 10

	// AFL files zip name
	aflzip = flag.String("aflzip", "test.zip", "Zip file name")
	// aflzip = "test.zip"

	//AFL files location
	afllocation = flag.String("afllocation", "/data/aflfiles/", "AFL files folder relative path")
	// afllocation = "/data/aflfiles/"

	queueFilesDir = flag.String("targetFilesDir", "/data/queuefiles", "Location of zip files of queue samples for sync")

	queueZip = flag.String("targetZip", "targetfiles.zip", "Zip containing queue files")

	dataDir = flag.String("dataDir", "/data", "Data directory")


)

var mutex *sync.Mutex

// Registration data from clients 
type RegMessage struct {
	MachineName string
	TargetName string
	Command string
}

var listenerCfg *utility.ListenerConfig

var comms *utility.CommChannels

func receiveWorker (comms *utility.CommChannels) {
	log.Println("receiveWorker started...")

	for {
		select {
		case message := <-comms.ReceiveSignal:
			switch string(message){
			case "":

			}
		}
	}
}

func unregWorker (comms *utility.CommChannels) {
	log.Println("unregWorker started...")
	for {
		select {
			case clientIp, ok := <- comms.UnregSignal:
				log.Println("Deleting client:", string(clientIp))
				if !ok {
					log.Println("Shutting down unregWorker")
					return
				}
				mutex.Lock()
				delete(clients, string(clientIp))
				mutex.Unlock()
		}
	}
}


func dispatcher (comms *utility.CommChannels) {
	log.Println("Dispatcher started...")
	ticker := time.NewTicker(time.Second * 4)
	defer ticker.Stop()
	quitkey := make (chan bool)
	for {
		select {
		case <- quitkey:
			ticker.Stop()
			log.Println("Main loop stopped")
			// Send stop signal to all clients
			comms.SendSignal <- []byte("Stop workers")

			close(comms.SendSignal)
			close(comms.ReceiveSignal)
		
		case <- ticker.C:
		// 1. Send stop signal
			log.Println("Send signal created")
			if len(clients) == 0 {
				log.Println("No clients connected!")
				continue
			}
			log.Println("Number of active clients:", len(clients))
			comms.SendSignal <- []byte("Stop workers")

		// 2. Wait for all files to be received - read loop
		// 3. Send files to clients
		// 4. Send start signal to clients
		}
	}
}

func init () {
	log.Println("Initializaing...")
	clients = make(map[string]*client.Client)
	sSignal := make (chan []byte)
	rSignal := make (chan []byte)
	uSignal := make (chan []byte)

	mutex = &sync.Mutex{}
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		Password: "", // no pass
		DB: 0, // Default DB
		})

	iniflags.Parse() // Parse flags 

	listenerCfg = &utility.ListenerConfig {
		CfgWriteWait: *writeWait, 
		CfgPongWait: *pongWait, 
		CfgMaxMessageSize: *maxMessageSize, 
		CfgPingPeriod: pingPeriod, 
		CfgAflZip: *aflzip, 
		CfgAflLocation: *afllocation,
		CfgDataDir: *dataDir,
		CfgQueueFilesDir: *queueFilesDir,
		CfgQueueZip: *queueZip,
	}

	comms = &utility.CommChannels {
		SendSignal: sSignal, 
		ReceiveSignal: rSignal, 
		UnregSignal: uSignal,
	}


}

func main() {
	// Initialization
	log.Println("Server listening on: " + addr + "...")

	// Start unregister thread
	go unregWorker (comms)

	// Start receiving thread
	go receiveWorker (comms)

	// Start dispatcher
	go dispatcher (comms)

	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/register", routes.RegisterHandler(redisClient))
	router.HandleFunc("/ws", routes.WebsocketHandler(redisClient, mutex, clients, listenerCfg, comms))
	router.HandleFunc("/aflfiles", routes.ServeAFLfiles(listenerCfg, redisClient))
	router.HandleFunc("/machinelist", routes.MachineList(redisClient, clients))
	router.HandleFunc("/queuefiles", routes.ServeQueueFiles(listenerCfg, redisClient))
	router.HandleFunc("/sendCommand", routes.SendCommand(listenerCfg, redisClient, clients))

	if err := http.ListenAndServe(addr, router); err != nil {
		panic(err)
	}

}