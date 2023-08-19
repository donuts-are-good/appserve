package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/syslog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type App struct {
	Server     *http.Server
	Routes     map[string]*Proxy
	RoutesFile string
	Mu         sync.RWMutex
}

type DomainRoute struct {
	Domain string `json:"domain"`
	Port   string `json:"port"`
}

type Proxy struct {
	Port  string
	Proxy *httputil.ReverseProxy
}
type SerializableProxy struct {
	Port   string `json:"port"`
	Domain string `json:"domain"`
}

func main() {
	logger, err := syslog.NewLogger(syslog.LOG_INFO|syslog.LOG_DAEMON, log.LstdFlags)
	if err != nil {
		log.Fatalf("Failed to initialize syslog logger: %v", err)
		return
	}
	log.SetOutput(logger.Writer())
	routesFile := "routes.json"

	app := &App{
		Routes:     make(map[string]*Proxy),
		RoutesFile: routesFile,
	}

	loadedRoutes, err := LoadRoutes(routesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatal(err)
		}
	} else {
		app.Routes = loadedRoutes
	}

	go app.startServer()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		args := strings.Fields(line)

		if len(args) == 0 {
			continue
		}

		switch args[0] {
		case "list":
			app.handleListCommand()
		case "add":
			if len(args) != 3 {
				fmt.Println("Error: Incorrect number of arguments. Expected: add <domain> <port>")
				continue
			}
			app.handleAddCommand(args[1], args[2])
		case "remove":
			if len(args) != 2 {
				fmt.Println("Error: Incorrect number of arguments. Expected: remove <domain>")
				continue
			}
			app.handleRemoveCommand(NormalizeDomain(args[1]))
		case "save":
			app.handleSaveCommand()
		case "load":
			app.handleLoadCommand()
		case "help":
			handleHelpCommand()
		case "exit":
			if app.Server != nil {
				log.Println("Initiating server shutdown...")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := app.Server.Shutdown(ctx); err != nil {
					log.Printf("Server shutdown encountered an error: %v", err)
				} else {
					log.Println("Server shutdown successfully.")
				}
			}
			log.Println("Exiting the application.")
			return

		default:
			fmt.Println("Error: Unknown command. Type 'help' for available commands.")
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from stdin: %v\n", err)
	}
}

func (app *App) startServer() {
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(app.getAllDomains()...), // Define your domain list here
		Cache:      autocert.DirCache("tls"),                       // Store certificates in the "tls" directory
	}

	server := &http.Server{
		Addr:      ":https",
		TLSConfig: certManager.TLSConfig(),
		Handler:   http.HandlerFunc(app.Handler()),
	}

	go func() {
		http.HandleFunc("/", app.Handler())
		// Serve on HTTP to satisfy the ACME HTTP-01 challenge and then redirect to HTTPS.
		log.Fatal(http.ListenAndServe(":http", certManager.HTTPHandler(nil)))
	}()

	log.Fatal(server.ListenAndServeTLS("", ""))
}

func (app *App) getAllDomains() []string {
	app.Mu.RLock()
	defer app.Mu.RUnlock()

	domains := make([]string, 0, len(app.Routes))
	for domain := range app.Routes {
		domains = append(domains, domain)
	}
	return domains
}

func LoadRoutes(file string) (map[string]*Proxy, error) {
	f, err := os.Open(file)
	if err != nil {
		log.Printf("Error opening file %s: %v", file, err)
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("Error closing file %s after reading: %v", file, cerr)
		}
	}()

	var routes []DomainRoute
	d := json.NewDecoder(f)
	if err := d.Decode(&routes); err != nil {
		return nil, fmt.Errorf("error decoding JSON from file %s: %w", file, err)
	}

	var failedRoutes int
	rp := make(map[string]*Proxy)
	for _, route := range routes {
		route.Domain = NormalizeDomain(route.Domain)
		log.Println("-> route found: " + route.Domain + ":" + route.Port)
		target, err := url.Parse("http://localhost:" + route.Port)
		if err != nil {
			log.Printf("Error parsing URL for domain %s: %v. Skipping this route.", route.Domain, err)
			failedRoutes++
			continue
		}
		rp[route.Domain] = &Proxy{
			Port:  route.Port,
			Proxy: httputil.NewSingleHostReverseProxy(target),
		}
	}

	if failedRoutes > 0 {
		log.Printf("%d routes failed to load due to errors.", failedRoutes)
	}

	return rp, nil
}

func SaveRoutes(file string, routes map[string]*Proxy) error {
	tempFile := file + ".tmp"

	f, err := os.Create(tempFile)
	if err != nil {
		log.Printf("Failed to create temporary file %s: %v", tempFile, err)
		return err
	}

	var serializableRoutes []SerializableProxy
	for domain, proxy := range routes {
		serializableRoutes = append(serializableRoutes, SerializableProxy{
			Port:   proxy.Port,
			Domain: domain,
		})
	}

	e := json.NewEncoder(f)
	err = e.Encode(serializableRoutes)
	if err != nil {
		log.Printf("Error while encoding JSON: %v", err)
		f.Close()
		os.Remove(tempFile)
		return err
	}

	if err := f.Close(); err != nil {
		log.Printf("Error while closing temporary file %s: %v", tempFile, err)
		os.Remove(tempFile)
		return err
	}

	if err := os.Rename(tempFile, file); err != nil {
		log.Printf("Failed to rename temporary file %s to %s: %v", tempFile, file, err)
		return err
	}

	return nil
}

func NormalizeDomain(domain string) string {
	return strings.TrimPrefix(strings.ToLower(domain), "www.")
}

func (app *App) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.Mu.RLock()
		defer app.Mu.RUnlock()

		domain := NormalizeDomain(r.Host)
		if route, found := app.Routes[domain]; found {
			fmt.Println("hit: ", domain, app.Routes[domain])
			route.Proxy.ServeHTTP(w, r)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}

func NewRoute(routes map[string]*Proxy, domain string, port string, mu *sync.RWMutex) error {
	domain = NormalizeDomain(domain)

	target, err := url.Parse("http://localhost:" + port)
	if err != nil {
		return err
	}

	routes[domain] = &Proxy{
		Port:  port,
		Proxy: httputil.NewSingleHostReverseProxy(target),
	}

	return nil
}

func handleHelpCommand() {
	fmt.Printf(`

Commands:
- list: List all of the domain-port mappings.
- add <domain> <port>: Add a mapping for the domain on the specified port.
    ex: add example.com 3000
- remove <domain>: Remove a mapping for the domain.
    ex: remove example.com
- save [filepath]: Save the routes to the specified filepath or default path if not specified.
- load [filepath]: Load routes from the specified filepath or default path if not specified.
- help: Show this help.
- exit: Exit the program.

`)
}

func (app *App) handleListCommand() {
	app.Mu.RLock()
	defer app.Mu.RUnlock()
	for domain, proxy := range app.Routes {
		fmt.Printf("Domain: %s, Port: %s\n", domain, proxy.Port)
	}
}

func (app *App) handleAddCommand(domain, port string) {
	app.Mu.Lock()
	defer app.Mu.Unlock()

	err := NewRoute(app.Routes, domain, port, &app.Mu)

	if err != nil {
		fmt.Printf("Error adding route: %v\n", err)
		return
	}

	err = SaveRoutes(app.RoutesFile, app.Routes)
	if err != nil {
		log.Println("Failed to save routes after adding:", err)
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Added new route for domain: %s on port: %s\n", domain, port)
}

func (app *App) handleRemoveCommand(domain string) {
	app.Mu.Lock()
	defer app.Mu.Unlock()
	if _, exists := app.Routes[domain]; !exists {
		fmt.Printf("Error: No such domain: %s\n", domain)
		return
	}
	delete(app.Routes, domain)
	err := SaveRoutes(app.RoutesFile, app.Routes)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Removed route for domain: %s\n", domain)
	}
}

func (app *App) handleSaveCommand() {
	err := SaveRoutes(app.RoutesFile, app.Routes)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Routes saved to: %s\n", app.RoutesFile)
	}
}

func (app *App) handleLoadCommand() error {
	routes, err := LoadRoutes(app.RoutesFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Error: No such file: %s\n", app.RoutesFile)
			return err
		} else {
			fmt.Printf("Error: %v\n", err)
			return err
		}
	}
	fmt.Printf("Routes loaded from: %s\n", app.RoutesFile)
	app.Mu.Lock()
	app.Routes = routes
	app.Mu.Unlock()
	return nil
}
