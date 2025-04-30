// Package server is a wrapper around the stdlib http server and x/autocert pkg.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	Log "log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"

	"golang.org/x/crypto/acme/autocert"
)

type listener struct {
	Addr     string `json:"addr"`
	FD       int    `json:"fd"`
	Filename string `json:"filename"`
}

func importListener(addr string) (net.Listener, error) {
	// Extract the encoded listener metadata from the environment.
	listenerEnv := os.Getenv("LISTENER")
	if listenerEnv == "" {
		return nil, fmt.Errorf("unable to find LISTENER environment variable")
	}

	// Unmarshal the listener metadata.
	var l listener
	err := json.Unmarshal([]byte(listenerEnv), &l)
	if err != nil {
		return nil, err
	}
	if l.Addr != addr {
		return nil, fmt.Errorf("unable to find listener for %v", addr)
	}

	// The file has already been passed to this process, extract the file
	// descriptor and name from the metadata to rebuild/find the *os.File for
	// the listener.
	listenerFile := os.NewFile(uintptr(l.FD), l.Filename)
	if listenerFile == nil {
		return nil, fmt.Errorf("unable to create listener file: %v", err)
	}
	defer listenerFile.Close()

	// Create a net.Listener from the *os.File.
	ln, err := net.FileListener(listenerFile)
	if err != nil {
		return nil, err
	}

	return ln, nil
}

func createListener(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return ln, nil
}

func createOrImportListener(addr string) (net.Listener, error) {
	// Try and import a listener for addr. If it's found, use it.
	ln, err := importListener(addr)
	if err == nil {
		log.Info(log.V{"msg": "Imported listener file descriptor for", "Address": addr})
		return ln, nil
	}

	// No listener was imported, that means this process has to create one.
	ln, err = createListener(addr)
	if err != nil {
		return nil, err
	}

	log.Info(log.V{"msg": "Created listener file descriptor for", "Address": addr})

	return ln, nil
}

func getListenerFile(ln net.Listener) (*os.File, error) {
	switch t := ln.(type) {
	case *net.TCPListener:
		return t.File()
	case *net.UnixListener:
		return t.File()
	}
	return nil, fmt.Errorf("unsupported listener: %T", ln)
}

func forkChild(addr string, ln net.Listener) (*os.Process, error) {
	// Get the file descriptor for the listener and marshal the metadata to pass
	// to the child in the environment.
	lnFile, err := getListenerFile(ln)
	if err != nil {
		return nil, err
	}
	defer lnFile.Close()
	l := listener{
		Addr:     addr,
		FD:       3,
		Filename: lnFile.Name(),
	}
	listenerEnv, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}

	// Pass stdin, stdout, and stderr along with the listener to the child.
	files := []*os.File{
		os.Stdin,
		os.Stdout,
		os.Stderr,
		lnFile,
	}

	// Get current environment and add in the listener to it.
	environment := append(os.Environ(), "LISTENER="+string(listenerEnv))

	// Get current process name and directory.
	execName, err := os.Executable()
	if err != nil {
		return nil, err
	}
	execDir := filepath.Dir(execName)

	// Spawn child process.
	p, err := os.StartProcess(execName, []string{execName}, &os.ProcAttr{
		Dir:   execDir,
		Env:   environment,
		Files: files,
		Sys:   &syscall.SysProcAttr{},
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

func waitForSignals(addr string, ln net.Listener, server *http.Server) error {
	signalCh := make(chan os.Signal, 1024)
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGUSR2, syscall.SIGINT, syscall.SIGQUIT)
	for {
		select {
		case s := <-signalCh:
			log.Info(log.V{"msg": "Received Signal", "Signal": s})
			switch s {
			case syscall.SIGHUP:
				// Fork a child process.
				p, err := forkChild(addr, ln)
				if err != nil {
					log.Error(log.V{"msg": "Unable to fork child", "Error": err})
					continue
				}
				log.Info(log.V{"msg": "Forked child", "Pid": p.Pid})

				// Create a context that will expire in 5 seconds and use this as a
				// timeout to Shutdown.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// Return any errors during shutdown.
				return server.Shutdown(ctx)
			case syscall.SIGUSR2:
				// Fork a child process.
				p, err := forkChild(addr, ln)
				if err != nil {
					log.Error(log.V{"msg": "Unable to fork child", "Error": err})
					continue
				}

				// Print the PID of the forked process and keep waiting for more signals.
				log.Info(log.V{"msg": "Forked child", "Pid": p.Pid})
			case syscall.SIGINT, syscall.SIGQUIT:
				// Create a context that will expire in 5 seconds and use this as a
				// timeout to Shutdown.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// Return any errors during shutdown.
				return server.Shutdown(ctx)
			}
		}
	}
}

// Server wraps the stdlib http server and x/autocert pkg with some setup.
type Server struct {

	// Which port to serve on - in 2.0 pass as argument for New()
	port int

	// Which mode we're in, read from ENV variable
	// Deprecated - due to be removed in 2.0
	production bool

	// Deprecated Logging - due to be removed in 2.0
	// Instead use the structured logging with server/log
	Logger Logger

	// Deprecated configs will be removed from the server object in 2.0
	// Use server/config instead to read the config from app.
	// Server configs - access with Config(string)
	configProduction  map[string]string
	configDevelopment map[string]string
	configTest        map[string]string
}

// New creates a new server instance
func New() (*Server, error) {

	// Check environment variable to see if we are in production mode
	prod := false
	if os.Getenv("FRAG_ENV") == "production" {
		prod = true
	}

	// Set up a new server
	s := &Server{
		port:              3000,
		production:        prod,
		configProduction:  make(map[string]string),
		configDevelopment: make(map[string]string),
		configTest:        make(map[string]string),
		Logger:            Log.New(os.Stderr, "fragmenta: ", Log.LstdFlags),
	}

	// Old style config read - this will be going away in Fragmenta 2.0
	// use server/config instead from the app
	err := s.readConfig()
	if err != nil {
		return s, err
	}
	err = s.readArguments()
	if err != nil {
		return s, err
	}

	return s, err
}

// Port returns the port of the server
func (s *Server) Port() int {
	return s.port
}

// PortString returns a string port suitable for passing to http.Server
func (s *Server) PortString() string {
	return fmt.Sprintf(":%d", s.port)
}

// watchProcess initialises OS signal and fork process
func watchProcess(server *http.Server, ln net.Listener) {
	go func() {
		time.Sleep(5 * time.Second)
		err := waitForSignals(server.Addr, ln, server)
		if err != nil {
			log.Error(log.V{"msg": "Exiting", "Error": err})
			return
		}
	}()
}

// Start starts an http server on the given port
func (s *Server) Start() error {
	server := &http.Server{
		// Set the port in the preferred string format
		Addr: s.PortString(),

		// The default server from net/http has no timeouts - set some limits
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       10 * time.Second, // IdleTimeout was introduced in Go 1.8

	}
	ln, err := createOrImportListener(server.Addr)
	if err != nil {
		log.Error(log.V{"msg": "Unable to create or import a listener", "Error": err})
		os.Exit(1)
	}
	if err == nil {
		watchProcess(server, ln)
	}

	return server.Serve(ln)
}

// StartTLS starts an https server on the given port
// with tls cert/key from config keys.
// Settings based on an article by Filippo Valsorda.
// https://blog.cloudflare.com/exposing-go-on-the-internet/
func (s *Server) StartTLS(cert, key string) error {

	// Set up a new http server
	server := &http.Server{
		// Set the port in the preferred string format
		Addr: s.PortString(),

		// The default server from net/http has no timeouts - set some limits
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       10 * time.Second, // IdleTimeout was introduced in Go 1.8

		// This TLS config follows recommendations in the above article
		TLSConfig: &tls.Config{
			// VersionTLS11 or VersionTLS12 would exclude many browsers
			// inc. Android 4.x, IE 10, Opera 12.17, Safari 6
			// So unfortunately not acceptable as a default yet
			// Current default here for clarity
			MinVersion: tls.VersionTLS10,

			// Causes servers to use Go's default ciphersuite preferences,
			// which are tuned to avoid attacks. Does nothing on clients.
			PreferServerCipherSuites: true,
			// Only use curves which have assembly implementations
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.X25519, // Go 1.8 only
			},
		},
	}

	return server.ListenAndServeTLS(cert, key)
}

// StartTLSAuto starts an https server on the given port
// by requesting certs from an ACME provider using the http-01 challenge.
// it also starts a server on the port 80 to listen for challenges and redirect
// The server must be on a public IP which matches the
// DNS for the domains.
func (s *Server) StartTLSAuto(email, domains string) error {
	autocertDomains := strings.Split(domains, " ")
	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      email,                                      // Email for projects with certs
		HostPolicy: autocert.HostWhitelist(autocertDomains...), // Domains to request certs for
		Cache:      autocert.DirCache("data/certs"),            // Cache certs in secrets folder
	}
	// Handle all :80 traffic using autocert to allow http-01 challenge responses
	go func() {
		http.ListenAndServe(":80", certManager.HTTPHandler(nil))
	}()

	server := s.ConfiguredTLSServer(certManager)
	ln, err := createOrImportListener(server.Addr)
	if err != nil {
		log.Error(log.V{"msg": "Unable to create or import a listener", "Error": err})
		os.Exit(1)
	}
	if err == nil {
		watchProcess(server, ln)
	}

	return server.ServeTLS(ln, "", "")
}

// StartTLSAutocert starts an https server on the given port
// by requesting certs from an ACME provider.
// The server must be on a public IP which matches the
// DNS for the domains.
func (s *Server) StartTLSAutocert(email string, domains string) error {
	autocertDomains := strings.Split(domains, " ")
	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      email,                                      // Email for projects with certs
		HostPolicy: autocert.HostWhitelist(autocertDomains...), // Domains to request certs for
		Cache:      autocert.DirCache("data/certs"),            // Cache certs in secrets folder
	}
	server := s.ConfiguredTLSServer(certManager)
	return server.ListenAndServeTLS("", "")
}

// ConfiguredTLSServer returns a TLS server instance with a secure config
// this server has read/write timeouts set to 20 seconds,
// prefers server cipher suites and only uses certain accelerated curves
// see - https://blog.gopheracademy.com/advent-2016/exposing-go-on-the-internet/
func (s *Server) ConfiguredTLSServer(certManager *autocert.Manager) *http.Server {

	return &http.Server{
		// Set the port in the preferred string format
		Addr: s.PortString(),

		// The default server from net/http has no timeouts - set some limits
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       10 * time.Second, // IdleTimeout was introduced in Go 1.8

		// This TLS config follows recommendations in the above article
		TLSConfig: &tls.Config{
			// Pass in a cert manager if you want one set
			// this will only be used if the server Certificates are empty
			GetCertificate: certManager.GetCertificate,

			// VersionTLS11 or VersionTLS12 would exclude many browsers
			// inc. Android 4.x, IE 10, Opera 12.17, Safari 6
			// So unfortunately not acceptable as a default yet
			// Current default here for clarity
			MinVersion: tls.VersionTLS10,

			// Causes servers to use Go's default ciphersuite preferences,
			// which are tuned to avoid attacks. Does nothing on clients.
			PreferServerCipherSuites: true,
			// Only use curves which have assembly implementations
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.X25519, // Go 1.8 only
			},
		},
	}

}

// StartRedirectAll starts redirecting all requests on the given port to the given host
// this should be called before StartTLS if redirecting http on port 80 to https
func (s *Server) StartRedirectAll(p int, host string) {
	port := fmt.Sprintf(":%d", p)
	// Listen and server on port p in a separate goroutine
	go func() {
		http.ListenAndServe(port, &redirectHandler{host: host})
	}()
}

// redirectHandler is useful if serving tls direct (not behind a proxy)
// and a redirect from port 80 is required.
type redirectHandler struct {
	host string
}

// ServeHTTP on this handler simply redirects to the main site
func (m *redirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, m.host+r.URL.String(), http.StatusMovedPermanently)
}
