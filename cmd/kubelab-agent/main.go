package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/natrontech/kubelab-agent/internal/log"
	"github.com/natrontech/kubelab-agent/pkg/xtermjs"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/spf13/cobra"
)

var VersionInfo string

func main() {
	if VersionInfo == "" {
		VersionInfo = "dev"
	}
	command := cobra.Command{
		Use:     "kubelab-agent",
		Short:   "Creates a web-based shell using xterm.js that links to an actual shell",
		Version: VersionInfo,
		RunE:    runE,
	}
	conf.ApplyToCobra(&command)
	command.Execute()
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if /scripts/check.sh exists
	if _, err := os.Stat("/scripts/check.sh"); os.IsNotExist(err) {
		http.Error(w, "Script does not exist", http.StatusInternalServerError)
		return
	}

	// Execute the shell script and capture combined output
	cmd := exec.Command("/bin/sh", "-c", "/scripts/check.sh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}

	// Check the exit status
	exitCode := cmd.ProcessState.ExitCode()
	if exitCode != 0 {
		http.Error(w, string(output), http.StatusInternalServerError)
		return
	}

	// Return HTTP response with status code 200 and "Everything OK" as response body
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Everything OK\n%s", output)
}

func bootstrap() error {
	// Check if /scripts/bootstrap.sh exists
	if _, err := os.Stat("/scripts/bootstrap.sh"); os.IsNotExist(err) {
		return err
	}

	// Execute the shell script
	cmd := exec.Command("/bin/sh", "-c", "/scripts/bootstrap.sh")
	err := cmd.Run()
	if err != nil {
		return err
	}

	// Check the exit status
	exitCode := cmd.ProcessState.ExitCode()
	if exitCode != 0 {
		return err
	}

	return nil
}

func bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if /scripts/bootstrap.sh exists
	if _, err := os.Stat("/scripts/bootstrap.sh"); os.IsNotExist(err) {
		http.Error(w, "Script does not exist", http.StatusInternalServerError)
		return
	}

	// Execute the shell script
	cmd := exec.Command("/bin/sh", "-c", "/scripts/bootstrap.sh")
	err := cmd.Run()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check the exit status
	exitCode := cmd.ProcessState.ExitCode()
	if exitCode != 0 {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return HTTP response with status code 200 and "Everything OK" as response body
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Everything OK")
}

func runE(_ *cobra.Command, _ []string) error {
	// initialise the logger
	log.Init(log.Format(conf.GetString("log-format")), log.Level(conf.GetString("log-level")))

	// debug stuff
	command := conf.GetString("command")
	connectionErrorLimit := conf.GetInt("connection-error-limit")
	arguments := conf.GetStringSlice("arguments")
	allowedHostnames := conf.GetStringSlice("allowed-hostnames")
	keepalivePingTimeout := time.Duration(conf.GetInt("keepalive-ping-timeout")) * time.Second
	maxBufferSizeBytes := conf.GetInt("max-buffer-size-bytes")
	pathLiveness := conf.GetString("path-liveness")
	pathMetrics := conf.GetString("path-metrics")
	pathReadiness := conf.GetString("path-readiness")
	pathXTermJS := conf.GetString("path-xtermjs")
	serverAddress := conf.GetString("server-addr")
	serverPort := conf.GetInt("server-port")
	workingDirectory := conf.GetString("workdir")
	if !path.IsAbs(workingDirectory) {
		wd, err := os.Getwd()
		if err != nil {
			message := fmt.Sprintf("failed to get working directory: %s", err)
			log.Error(message)
			return errors.New(message)
		}
		workingDirectory = path.Join(wd, workingDirectory)
	}
	log.Infof("working directory     : '%s'", workingDirectory)
	log.Infof("command               : '%s'", command)
	log.Infof("arguments             : ['%s']", strings.Join(arguments, "', '"))

	log.Infof("allowed hosts         : ['%s']", strings.Join(allowedHostnames, "', '"))
	log.Infof("connection error limit: %v", connectionErrorLimit)
	log.Infof("keepalive ping timeout: %v", keepalivePingTimeout)
	log.Infof("max buffer size       : %v bytes", maxBufferSizeBytes)
	log.Infof("server address        : '%s' ", serverAddress)
	log.Infof("server port           : %v", serverPort)

	log.Infof("liveness checks path  : '%s'", pathLiveness)
	log.Infof("readiness checks path : '%s'", pathReadiness)
	log.Infof("metrics endpoint path : '%s'", pathMetrics)
	log.Infof("xtermjs endpoint path : '%s'", pathXTermJS)

	// configure routing
	router := mux.NewRouter()

	// this is the endpoint for xterm.js to connect to
	xtermjsHandlerOptions := xtermjs.HandlerOpts{
		AllowedHostnames:     allowedHostnames,
		Arguments:            arguments,
		Command:              command,
		ConnectionErrorLimit: connectionErrorLimit,
		CreateLogger: func(connectionUUID string, r *http.Request) xtermjs.Logger {
			createRequestLog(r, map[string]interface{}{"connection_uuid": connectionUUID}).Infof("created logger for connection '%s'", connectionUUID)
			return createRequestLog(nil, map[string]interface{}{"connection_uuid": connectionUUID})
		},
		KeepalivePingTimeout: keepalivePingTimeout,
		MaxBufferSizeBytes:   maxBufferSizeBytes,
	}
	router.HandleFunc(pathXTermJS, xtermjs.GetHandler(xtermjsHandlerOptions))

	// readiness probe endpoint
	router.HandleFunc(pathReadiness, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// liveness probe endpoint
	router.HandleFunc(pathLiveness, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// metrics endpoint
	router.Handle(pathMetrics, promhttp.Handler())

	// version endpoint
	router.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(VersionInfo))
	})

	// check endpoint
	router.Handle("/check", http.HandlerFunc(checkHandler))

	// bootstrap endpoint
	router.Handle("/bootstrap", http.HandlerFunc(bootstrapHandler))

	// this is the endpoint for serving xterm.js assets
	depenenciesDirectory := path.Join(workingDirectory, "./node_modules")
	router.PathPrefix("/assets").Handler(http.StripPrefix("/assets", http.FileServer(http.Dir(depenenciesDirectory))))

	// this is the endpoint for the root path aka website
	publicAssetsDirectory := path.Join(workingDirectory, "./public")
	router.PathPrefix("/").Handler(http.FileServer(http.Dir(publicAssetsDirectory)))

	// start memory logging pulse
	logWithMemory := createMemoryLog()
	go func(tick *time.Ticker) {
		for {
			logWithMemory.Debug("tick")
			<-tick.C
		}
	}(time.NewTicker(time.Second * 30))

	// create a CORS handler
	c := cors.New(cors.Options{
		AllowedOrigins:   allowedHostnames, // AllowedOrigins is a list of origins a cross-domain request can be executed from
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowCredentials: true,
		AllowedHeaders:   []string{"Authorization", "Content-Type"}, // Your server headers here...
	})

	// wrap your router with CORS handler
	handler := c.Handler(router)

	// listen
	listenOnAddress := fmt.Sprintf("%s:%v", serverAddress, serverPort)
	server := http.Server{
		Addr:    listenOnAddress,
		Handler: addIncomingRequestLogging(handler),
	}

	// bootstrap
	if err := bootstrap(); err != nil {
		log.Errorf("failed to bootstrap: %s", err)
		return err
	} else {
		log.Info("bootstrapped")
	}

	log.Infof("starting server on interface:port '%s'...", listenOnAddress)
	return server.ListenAndServe()
}
