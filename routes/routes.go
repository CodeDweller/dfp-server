package routes

import (
	//"fmt"
	"log"
	"net/http"
	"strconv"
	"net"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/go-redis/redis"
	"ADFPserver/client"
	"ADFPserver/utility"

)

var upgrader = websocket.Upgrader{}

func RegisterHandler (redisClient *redis.Client) func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		log.Println("registerHandler")
		if r.Method != "POST" {
			log.Println("Wrong requests method!")
			http.Error(w, "Wrong req method", http.StatusBadRequest)
			return
		}

		var clientData utility.ClientInfo
		if r.Body == nil {
			http.Error(w,"Request body empty", 400)
			return
		}

		err := json.NewDecoder(r.Body).Decode(&clientData)
		log.Println("Client data:", clientData)

		ip, _, err := net.SplitHostPort(r.RemoteAddr)

		// Find the machine in Redis
		machine, err := redisClient.HGet(ip, "machineName").Result()
		log.Println("Machine: ", machine)
		if err == redis.Nil {
			// Client doesn't exist, create a new name, insert in Redis, send current task
			machinenum, err := redisClient.Incr("ClientNum").Result()
			log.Println("New machine number: ", machinenum)
			if err != nil {
				log.Println("Redis: Unable to increment clientnum")
				http.Error(w, "Redis: Unable to increment clientnum" + err.Error(), http.StatusInternalServerError)
				return
			}
			log.Println("Client not found in Redis: adding new client")

			// Create map for Redis insertion
			machinename := "Machine" + strconv.Itoa(int(machinenum))
			//machineType := redisClient.HGet(ip, "OSversion")
			currTarget, err := redisClient.HGet("currentTargetName", "linux").Result()
			//currentCommand, err := redisClient.Get("currentCommand").Result()
			targetOffset, err := redisClient.Get("targetOffset").Result()
			tempMap := map[string]interface{} {
				"machineName" : machinename,
				"currentTarget" : currTarget,
				//"currentCommand" : currentCommand,
				"instancesNum" : clientData.LogicalCoresNum,
				"OSversion" : clientData.OperatingSystem,
				"targetOffset" : targetOffset,
			}
			err = redisClient.HMSet(ip, tempMap).Err()
			if err != nil {
				log.Println("Unable to insert into Redis: ", err)
				http.Error(w, "Unable to insert into Redis: " + err.Error(), http.StatusInternalServerError)
				return
			}

			// Create JSON
			if err != nil {
				log.Println("Unable to marshal JSON")
				http.Error(w, "Unable to marshal JSON: " + err.Error(), http.StatusInternalServerError)
				return
			}
			tempStringMap := map[string]string {
				"machineName": machinename, 
				"currentTarget": currTarget, 
				//"currentCommand": currentCommand, 
				"targetOffset": targetOffset, 
				"firstContact": "true",
			}
		    w.Header().Set("Content-Type", "application/json")
		    w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(tempStringMap)
			return

		} else if err != nil {
			log.Println("Error reading from Redis: ", err)
			http.Error(w, "Error reading from Redis! " + err.Error(), http.StatusInternalServerError)
			return
		}

		// Machine is already registered
		log.Println("Redis: IP already exists")
		currTarget, err := redisClient.HGet(ip, "currentTarget").Result()
		currentCommand, err := redisClient.HGet(ip, "currentCommand").Result()

		if err != nil {
			log.Println("Unable to marshal JSON")
			http.Error(w, "Unable to marshal JSON: " + err.Error(), http.StatusInternalServerError)
			return
		}

		tempStringMap := map[string]string {
			"machineName": string(machine), 
			"currentTarget": string(currTarget), 
			"currentCommand": string(currentCommand), 
			"firstContact": "false", 
		}
	    w.Header().Set("Content-Type", "application/json")
	 	w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tempStringMap)
		return		
	}

}

func WebsocketHandler (redisClient *redis.Client, mutex *sync.Mutex, clients map[string]*client.Client, cfg *utility.ListenerConfig, comms *utility.CommChannels) func (w http.ResponseWriter, r *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		log.Println("Packet received: ", w)
		log.Println("Creating WS connection...")
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		_, err = redisClient.HGet(ip, "machineName").Result()
		if err == redis.Nil {
			log.Println("Redis: Machine not registered")
			http.Error(w, "Machine not registered! ", http.StatusForbidden)
			return
		} else if err != nil {
			log.Println("Error reading from Redis:", err)
			http.Error(w, "Error reading from Redis! " + err.Error(), http.StatusInternalServerError)
			return
		}
		// Create websocket with client
		//createClientListener(w, r, ip, mutex, clients, cfg)
		log.Println("Upgrading to WS...")
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade error: ", err)
			return
		}
		err = createClientListener(c, ip, mutex, clients, cfg, comms)
		return
	}
}

func SendCommand (listenerCfg *utility.ListenerConfig, redisClient *redis.Client, clients map[string]*client.Client) func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		log.Println("Sending command...")
		if r.Method != "GET" {
			log.Println("Wrong requests method!")
			http.Error(w, "Wrong req method", http.StatusBadRequest)
			return
		}

		// ip, _, err := net.SplitHostPort(r.RemoteAddr)
	
		command, err := redisClient.HGetAll("WinCmd").Result()
		if err != nil {
			log.Println("Redis error")
			http.Error(w, "Redis error!", http.StatusInternalServerError)
			return		
		}
	 	w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(command)
		return		
	}
}

func ServeAFLfiles (listenerCfg *utility.ListenerConfig, redisClient *redis.Client) func (w http.ResponseWriter, r *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		log.Println("Sending AFL files...")
		cwd, _ := os.Getwd()

		// Check OS version
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		osVersion, err := redisClient.HGet(ip, "OSversion").Result()
		if err != nil {
			log.Println("Error reading from Redis:", err)
			http.Error(w, "Error reading from Redis! " + err.Error(), http.StatusInternalServerError)
		}
		log.Println("Client OS version:", osVersion)
		if osVersion == "linux" {
			log.Println("File to serve:", cwd + listenerCfg.CfgAflLocation + "linux/" + "afl-fuzz.zip")
			http.ServeFile(w, r, cwd + listenerCfg.CfgAflLocation + "linux/" + "afl-fuzz.zip")
		} else {
			http.ServeFile(w, r, cwd + listenerCfg.CfgAflLocation + "windows/" + listenerCfg.CfgAflZip)
		}
		log.Println("Files served!")
	}
}

func MachineList (redisClient *redis.Client, clients map[string]*client.Client) func (w http.ResponseWriter, r *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		log.Println("Returning machine list...")
		if r.Method != "GET" {
			log.Println("Wrong requests method!")
			http.Error(w, "Wrong req method", http.StatusBadRequest)
			return
		}
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		type MachineData struct {
			MachineName string
			InstanceNum string
		}
		var machines []MachineData
		machineType := redisClient.HGet(ip, "OSversion")
		// Retrive all machine names from Redis
		for mIp, _ := range clients {
			tempName, err := redisClient.HGet(mIp, "machineName").Result()
			tempCoreNum, err := redisClient.HGet(mIp, "instancesNum").Result()
			if err != nil {
				log.Println("Unable to retrieve list of machines from Redis:", err.Error())
				http.Error(w, "Unable to retrieve list of machines from Redis! " + err.Error(), http.StatusInternalServerError)
				return
			}
			// Don't send it's own IP
			if (mIp == ip) { continue } 
			// Send only machines with same OS as the machine requesting
			machineListType := redisClient.HGet(mIp, "OSversion")
			if (machineType != machineListType) { continue }

			machines = append(machines, MachineData{MachineName: tempName, InstanceNum: tempCoreNum})
		}
		log.Println("Machine list:", machines)
		// If list is empty, notify client
		if (len(machines) == 0) {
			log.Println("Machine list empty")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("No registered machines"))
			return
		}
		jData, err := json.Marshal(machines)
		if err != nil {
			log.Println("Unable to marshal machines slice")
			http.Error(w, "Unable to retrieve list of machines from Redis! " + err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jData)
		return
	}
}

func ServeQueueFiles (listenerCfg *utility.ListenerConfig, redisClient *redis.Client) func (w http.ResponseWriter, r *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		log.Println("Sending Target files...")
		cwd, _ := os.Getwd()
		targetDir:= filepath.Join(cwd, listenerCfg.CfgQueueFilesDir, "testTargetName") // Name for mock use only
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		machine, err := redisClient.HGet(ip, "machineName").Result()
		if err != nil {
			log.Println("Error reading from Redis: ", err)
			http.Error(w, "Error reading from Redis! " + err.Error(), http.StatusInternalServerError)
			return			
		}

		log.Println("Machine name:", machine)
		// globExclude := "!(*" + machine + ").zip" // Doesn't work

		fls := filepath.Join(targetDir, "*.zip")
		log.Println("Glob:", fls)

		// Pack all received zip files into single zip file
		files, err := filepath.Glob(fls)
		if err != nil {
			log.Println("ServeTargetFiles can't Glob", err)
		}
		// Remove zip of machine that requested files
		for i, file := range files {
			if file == filepath.Join(targetDir, machine + ".zip") {
				files[i] = files[len(files)-1]
				files = files[:len(files)-1]	
			}
		}
		log.Println("Queue file list:", files)

		zipfile := filepath.Join(cwd, listenerCfg.CfgDataDir)
		log.Println("File to serve:", zipfile)

		err = utility.ZipFiles(filepath.Join(zipfile, listenerCfg.CfgQueueZip), files)
		if err != nil {
			log.Println("Can't zip files: ", err)
		}

		http.ServeFile(w, r, filepath.Join(zipfile, listenerCfg.CfgQueueZip))
		log.Println("Files served!")		
	}
}


func createClientListener (conn *websocket.Conn, ip string, mutex *sync.Mutex, clients map[string]*client.Client, cfg *utility.ListenerConfig, comms *utility.CommChannels) error {

	// Add new client to client list
	newClient := &client.Client{
		Ip: ip, 
		Conn: conn, 
		IsActive: true, 
		WriteWait: cfg.CfgWriteWait, 
		PongWait: cfg.CfgPongWait, 
		MaxMessageSize: cfg.CfgMaxMessageSize, 
		PingPeriod: cfg.CfgPingPeriod,
	}
	mutex.Lock()
	clients[ip] = newClient
	mutex.Unlock()

	log.Println("New client in map:", clients[ip])
	go clients[ip].ClientReader(comms.ReceiveSignal, comms.UnregSignal)
	go clients[ip].ClientWriter(comms.SendSignal)

	return nil
}